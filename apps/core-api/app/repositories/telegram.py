"""Persistence operations for Telegram signal ingestion."""

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models import ChatGroup, Signal, SystemLog


class TelegramRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def signal_context(self, chat_id: str) -> tuple[str, list[str]] | None:
        group = self._session.get(ChatGroup, chat_id)
        if group is None or not group.enabled:
            return None
        recent = self._session.scalars(
            select(Signal.raw_text)
            .where(Signal.group_id == chat_id)
            .order_by(Signal.created_at.desc(), Signal.id.desc())
            .limit(5)
        ).all()
        return group.alias or group.name, [text for text in recent if text]

    def log_processing_failure(self, exception_type: str) -> None:
        self._session.add(
            SystemLog(
                level="ERROR",
                source="Telegram",
                message=f"Message processing failed: {exception_type}",
            )
        )
        self._session.commit()

    def saved_groups(self) -> list[ChatGroup]:
        return list(self._session.scalars(select(ChatGroup)))

    def save_group_selections(
        self,
        selections: list[dict[str, object]],
        available_groups: dict[str, dict[str, object]],
    ) -> int:
        submitted_ids = {str(item["chat_id"]) for item in selections}
        groups = self.saved_groups()
        disabled = 0
        for saved in groups:
            if saved.enabled and saved.chat_id not in submitted_ids:
                saved.enabled = False
                disabled += 1
        saved_by_id = {item.chat_id: item for item in groups}
        for selection in selections:
            chat_id = str(selection["chat_id"])
            if chat_id not in available_groups:
                raise ValueError("Grup harus dipilih dari daftar akun Telegram")
            item = saved_by_id.get(chat_id)
            if item is None:
                item = ChatGroup(chat_id=chat_id, name=str(available_groups[chat_id]["name"]))
                self._session.add(item)
                saved_by_id[chat_id] = item
            item.enabled = bool(selection["enabled"])
            alias = selection.get("alias")
            item.alias = alias.strip()[:255] if isinstance(alias, str) and alias.strip() else None
        self._session.commit()
        return disabled
