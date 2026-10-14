"""Business rules for approving or rejecting submitted payment proofs."""

import hashlib
import secrets

from sqlalchemy.orm import Session

from ..domain.client_plans import resolve_plan
from ..models import MT5Account
from ..repositories.payments import PaymentRepository


def review_payment_order(db: Session, order_id: int, decision: str) -> tuple[str, int | None]:
    if decision not in {"accept", "invalid"}:
        raise ValueError("Keputusan pembayaran tidak dikenal")
    repository = PaymentRepository(db)
    order = repository.lock_for_review(order_id)
    if order is None:
        repository.rollback()
        return "MISSING", None
    if order.status != "PAYMENT_SUBMITTED":
        repository.rollback()
        return order.status, order.account_id

    if decision == "invalid":
        repository.reject_payment(order)
        return "REJECTED", None

    plan = resolve_plan(order.plan)
    if plan is None or order.total_idr != plan["price_idr"]:
        repository.rollback()
        return "INVALID_ORDER", None
    duplicate_id = repository.duplicate_account_id(
        server=order.server,
        login=order.login,
        owner_id=order.owner_id,
        managed=order.execution_mode == "MANAGED",
    )
    if duplicate_id is not None:
        repository.rollback()
        return "ACCOUNT_EXISTS", duplicate_id

    token = secrets.token_urlsafe(32)
    account = MT5Account(
        owner_id=order.owner_id,
        label=order.label,
        server=order.server,
        login=order.login,
        plan=order.plan,
        execution_mode=order.execution_mode,
        role="follower",
        token_hash=hashlib.sha256(token.encode("utf-8")).hexdigest(),
    )
    if not repository.approve_payment(order, account):
        return "ACCOUNT_EXISTS", None
    return "APPROVED", account.id
