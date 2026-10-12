"""Persistence operations for client payment orders."""

from sqlalchemy import func, select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from ..models import MT5Account, PaymentOrder, utc_now
from .transactions import begin_serialized_write


ACTIVE_ORDER_STATUSES = ("PENDING_PAYMENT", "PAYMENT_SUBMITTED")


class PaymentRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def list_for_owner(self, owner_id: int) -> list[PaymentOrder]:
        return list(
            self._session.scalars(
                select(PaymentOrder).where(PaymentOrder.owner_id == owner_id).order_by(PaymentOrder.id.desc()).limit(20)
            )
        )

    def get_for_owner(self, order_id: int, owner_id: int) -> PaymentOrder | None:
        return self._session.execute(
            select(PaymentOrder).where(PaymentOrder.id == order_id, PaymentOrder.owner_id == owner_id)
        ).scalar_one_or_none()

    def get_by_id(self, order_id: int) -> PaymentOrder | None:
        return self._session.get(PaymentOrder, order_id)

    def rollback(self) -> None:
        self._session.rollback()

    def lock_for_review(self, order_id: int) -> PaymentOrder | None:
        begin_serialized_write(self._session, f"payment-order:{order_id}")
        query = select(PaymentOrder).where(PaymentOrder.id == order_id)
        if self._session.get_bind().dialect.name != "sqlite":
            query = query.with_for_update()
        return self._session.execute(query).scalar_one_or_none()

    def duplicate_account_id(self, *, server: str, login: str, owner_id: int, managed: bool) -> int | None:
        query = select(MT5Account.id).where(
            func.lower(MT5Account.server) == server.casefold(),
            MT5Account.login == login,
        )
        if not managed:
            query = query.where(MT5Account.owner_id == owner_id)
        return self._session.execute(query).scalar_one_or_none()

    def reject_payment(self, order: PaymentOrder) -> None:
        order.status = "REJECTED"
        order.updated_at = utc_now()
        self._session.commit()

    def approve_payment(self, order: PaymentOrder, account: MT5Account) -> bool:
        self._session.add(account)
        self._session.flush()
        order.account_id = account.id
        order.status = "APPROVED"
        order.updated_at = utc_now()
        try:
            self._session.commit()
        except IntegrityError:
            self._session.rollback()
            return False
        return True

    def mark_admin_notified(self, order_id: int) -> None:
        order = self.get_by_id(order_id)
        if order is not None and order.status == "PAYMENT_SUBMITTED":
            order.admin_notified_at = utc_now()
            self._session.commit()

    def cancel_for_owner(self, order_id: int, owner_id: int) -> tuple[PaymentOrder | None, str | None, bool]:
        is_sqlite = self._session.get_bind().dialect.name == "sqlite"
        begin_serialized_write(self._session, f"payment-order:{order_id}")
        query = select(PaymentOrder).where(
            PaymentOrder.id == order_id,
            PaymentOrder.owner_id == owner_id,
        )
        if not is_sqlite:
            query = query.with_for_update()
        order = self._session.execute(query).scalar_one_or_none()
        if order is None:
            self._session.rollback()
            return None, None, False
        if order.status not in ACTIVE_ORDER_STATUSES:
            self._session.rollback()
            return order, None, False
        old_proof = order.proof_filename
        order.status = "CANCELLED"
        order.updated_at = utc_now()
        self._session.commit()
        return order, old_proof, True
