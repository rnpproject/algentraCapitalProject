"""Client account levels offered during registration and billing."""

from ..core.config import get_settings

# Paket lama (ZERO/PRO/EXPERT): dipertahankan agar order dan akun yang sudah
# ada tetap tampil dengan benar. Order baru memakai model harga layanan.
CLIENT_PLANS: dict[str, dict[str, str | int]] = {
    "ZERO": {"name": "ZERO", "vcpu": "1 core", "ram": "1 GB", "disk": "20 GB", "price_idr": 150_000},
    "PRO": {"name": "PRO", "vcpu": "2 core", "ram": "4 GB", "disk": "40 GB", "price_idr": 200_000},
    "EXPERT": {"name": "EXPERT", "vcpu": "4 core", "ram": "8 GB", "disk": "80 GB", "price_idr": 300_000},
}

VPS_SLOTS = 5


def get_vps_options() -> list[dict[str, str | int]]:
    """Daftar VPS dari .env (VPS{n}_NAME/VCPU/RAM/DISK/PRICE_IDR).

    Slot tanpa VPS{n}_NAME dianggap tidak tersedia dan dilewati.
    """
    settings = get_settings()
    options: list[dict[str, str | int]] = []
    for i in range(1, VPS_SLOTS + 1):
        name = str(getattr(settings, f"vps{i}_name", "") or "").strip()
        if not name:
            continue
        price = int(getattr(settings, f"vps{i}_price_idr", 0) or 0)
        options.append(
            {
                "code": f"VPS{i}",
                "name": name,
                "vcpu": str(getattr(settings, f"vps{i}_vcpu", "") or "").strip() or "—",
                "ram": str(getattr(settings, f"vps{i}_ram", "") or "").strip() or "—",
                "disk": str(getattr(settings, f"vps{i}_disk", "") or "").strip() or "—",
                "price_idr": max(price, 0),
            }
        )
    return options


def get_service_pricing() -> dict[str, object]:
    """Harga layanan dari .env (single source of truth).

    - management_fee_idr: biaya pengelolaan akun (mode MANAGED).
    - copy_trading_fee_idr: biaya copy trading (mode EA) = setengah management fee.
    - vps_options: daftar VPS yang dikonfigurasi (kosong bila tidak ada).
    """
    settings = get_settings()
    management_fee = int(settings.management_fee_idr)
    return {
        "management_fee_idr": management_fee,
        "copy_trading_fee_idr": management_fee // 2,
        "vps_options": get_vps_options(),
    }


def get_service_plans() -> dict[str, dict[str, str | int]]:
    """Kode plan model baru untuk order dan tampilan."""
    pricing = get_service_pricing()
    copy_fee = int(pricing["copy_trading_fee_idr"])
    plans: dict[str, dict[str, str | int]] = {
        "MANAGED": {
            "name": "Dikelola server",
            "vcpu": "—",
            "ram": "—",
            "disk": "—",
            "price_idr": int(pricing["management_fee_idr"]),
        },
        "EA": {
            "name": "EA sendiri",
            "vcpu": "—",
            "ram": "—",
            "disk": "—",
            "price_idr": copy_fee,
        },
    }
    vps_options = pricing["vps_options"]
    assert isinstance(vps_options, list)
    for vps in vps_options:
        code = f"EA_{vps['code']}"
        plans[code] = {
            "name": f"EA sendiri + {vps['name']}",
            "vcpu": str(vps["vcpu"]),
            "ram": str(vps["ram"]),
            "disk": str(vps["disk"]),
            "price_idr": copy_fee + int(vps["price_idr"]),
        }
    return plans


def resolve_plan(code: str) -> dict[str, str | int] | None:
    """Cari definisi plan dari kode, mencakup paket lama dan model baru."""
    if code in CLIENT_PLANS:
        return CLIENT_PLANS[code]
    return get_service_plans().get(code)
