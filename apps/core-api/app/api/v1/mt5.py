from fastapi import APIRouter

from ...services import mt5_service


router = APIRouter(prefix="/mt5", tags=["mt5"])

# Keep these imports available for existing internal callers while their
# implementation lives outside the HTTP router.
require_client_id = mt5_service.require_client_id
_token_hash = mt5_service._token_hash
__all__ = ["router", "require_client_id", "_token_hash"]

router.add_api_route("/accounts", mt5_service.list_accounts, methods=["GET"])
router.add_api_route("/plans", mt5_service.list_client_plans, methods=["GET"])
router.add_api_route("/pricing", mt5_service.service_pricing, methods=["GET"])
router.add_api_route("/accounts/{account_id}/managed-copy", mt5_service.configure_managed_copy, methods=["PUT"])
router.add_api_route("/accounts/{account_id}/managed-copy/active", mt5_service.set_managed_copy_active, methods=["PUT"])
router.add_api_route("/copier/jobs", mt5_service.list_managed_copy_jobs, methods=["GET"])
router.add_api_route("/copier/health", mt5_service.managed_copy_worker_health, methods=["GET"])
router.add_api_route(
    "/copier/jobs/{account_id}/login-check", mt5_service.report_managed_copy_login_check, methods=["POST"]
)
router.add_api_route("/copier/status", mt5_service.managed_copy_service_status, methods=["GET"])
router.add_api_route(
    "/copier/jobs/{account_id}/status", mt5_service.report_managed_copy_worker_status, methods=["POST"]
)
router.add_api_route("/performance", mt5_service.client_mt5_performance, methods=["GET"])
router.add_api_route("/accounts", mt5_service.create_account, methods=["POST"])
router.add_api_route("/accounts/{account_id}/rotate-token", mt5_service.rotate_token, methods=["POST"])
router.add_api_route("/accounts/{account_id}/active", mt5_service.set_active, methods=["PUT"])
router.add_api_route("/accounts/{account_id}", mt5_service.update_account, methods=["PUT"])
router.add_api_route("/accounts/{account_id}", mt5_service.delete_account, methods=["DELETE"])
router.add_api_route("/follower/report", mt5_service.report_follower_account, methods=["POST"])
router.add_api_route("/copier/jobs/{account_id}/report", mt5_service.report_managed_copy_account, methods=["POST"])
router.add_api_route("/follower/positions", mt5_service.follower_positions, methods=["GET"])
