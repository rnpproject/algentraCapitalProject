"""Conversation and account-context persistence for customer service chat."""

from datetime import timedelta

from sqlalchemy import delete, select
from sqlalchemy.orm import Session

from ..models import CSChatMessage, CSConversation, ClientUser, MT5Account, utc_now


class CustomerServiceRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def client_exists(self, client_id: int) -> bool:
        return self._session.get(ClientUser, client_id) is not None

    def expire_conversations(self, now=None) -> None:
        self._session.execute(delete(CSConversation).where(CSConversation.expires_at <= (now or utc_now())))
        self._session.flush()

    def active_conversation(
        self,
        *,
        client_id: int | None,
        visitor_hash: str | None,
        now,
    ) -> CSConversation | None:
        query = select(CSConversation).where(CSConversation.expires_at > now)
        if client_id is not None:
            query = query.where(CSConversation.client_user_id == client_id)
        else:
            query = query.where(CSConversation.public_visitor_hash == visitor_hash)
        return self._session.execute(
            query.order_by(CSConversation.started_at.desc(), CSConversation.id.desc()).limit(1)
        ).scalar_one_or_none()

    def get_or_create_conversation(
        self,
        *,
        client_id: int | None,
        visitor_hash: str | None,
        lifetime: timedelta,
    ) -> CSConversation:
        now = utc_now()
        self.expire_conversations(now)
        conversation = self.active_conversation(client_id=client_id, visitor_hash=visitor_hash, now=now)
        if conversation is None:
            conversation = CSConversation(
                client_user_id=client_id,
                public_visitor_hash=visitor_hash,
                started_at=now,
                expires_at=now + lifetime,
            )
            self._session.add(conversation)
            self._session.commit()
            self._session.refresh(conversation)
        else:
            self._session.commit()
        return conversation

    def messages(self, conversation_id: int, limit: int) -> list[CSChatMessage]:
        return list(
            self._session.scalars(
                select(CSChatMessage)
                .where(CSChatMessage.conversation_id == conversation_id)
                .order_by(CSChatMessage.created_at.desc(), CSChatMessage.id.desc())
                .limit(limit)
            )
        )

    def owned_accounts(self, client_id: int) -> list[MT5Account]:
        return list(
            self._session.scalars(
                select(MT5Account).where(MT5Account.owner_id == client_id).order_by(MT5Account.id.asc())
            )
        )

    def owned_active_account(self, account_id: int, client_id: int) -> MT5Account | None:
        return self._session.execute(
            select(MT5Account).where(
                MT5Account.id == account_id,
                MT5Account.owner_id == client_id,
                MT5Account.active.is_(True),
            )
        ).scalar_one_or_none()

    def save_exchange(self, conversation_id: int, user_message: str, assistant_message: str) -> None:
        now = utc_now()
        self._session.add_all(
            [
                CSChatMessage(conversation_id=conversation_id, role="user", content=user_message, created_at=now),
                CSChatMessage(
                    conversation_id=conversation_id, role="assistant", content=assistant_message, created_at=now
                ),
            ]
        )
        self._session.commit()

    def commit(self) -> None:
        self._session.commit()
