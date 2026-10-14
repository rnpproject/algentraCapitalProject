"""HTTP endpoints for authentication workflows."""

from fastapi import APIRouter, BackgroundTasks, Depends, Request, status
from sqlalchemy.orm import Session

from ...database import get_db
from ...schemas import (
    ClientLoginRequest,
    LoginRequest,
    PasswordResetConfirmRequest,
    PasswordResetRequest,
    RegisterRequest,
    ResendCodeRequest,
    VerifyEmailRequest,
)
from ...services import auth_service

router = APIRouter(prefix="/auth", tags=["auth"])


@router.post("/login")
def login(payload: LoginRequest, request: Request, db: Session = Depends(get_db)) -> dict[str, str]:
    return auth_service.login(payload, request, db)


@router.post("/register", status_code=status.HTTP_202_ACCEPTED)
def register(payload: RegisterRequest, db: Session = Depends(get_db)) -> dict[str, str | int]:
    return auth_service.register(payload, db)


@router.post("/resend-code", status_code=status.HTTP_202_ACCEPTED)
def resend_code(payload: ResendCodeRequest, db: Session = Depends(get_db)) -> dict[str, str | int]:
    return auth_service.resend_code(payload, db)


@router.post("/verify-email")
def verify_email(payload: VerifyEmailRequest, request: Request, db: Session = Depends(get_db)) -> dict[str, str | int]:
    return auth_service.verify_email(payload, request, db)


@router.post("/password-reset/request", status_code=status.HTTP_202_ACCEPTED)
def request_password_reset(
    payload: PasswordResetRequest,
    background_tasks: BackgroundTasks,
    db: Session = Depends(get_db),
) -> dict[str, str | int]:
    return auth_service.request_password_reset(payload, background_tasks, db)


@router.post("/password-reset/confirm")
def confirm_password_reset(payload: PasswordResetConfirmRequest, db: Session = Depends(get_db)) -> dict[str, str]:
    return auth_service.confirm_password_reset(payload, db)


@router.post("/client-login")
def client_login(payload: ClientLoginRequest, request: Request, db: Session = Depends(get_db)) -> dict[str, str | int]:
    return auth_service.client_login(payload, request, db)


@router.post("/logout")
def logout(request: Request) -> dict[str, str]:
    return auth_service.logout(request)


@router.get("/session")
def session_status(request: Request) -> dict[str, bool]:
    return {
        "admin": request.session.get("admin_username") is not None,
        "client": request.session.get("client_user_id") is not None,
    }
