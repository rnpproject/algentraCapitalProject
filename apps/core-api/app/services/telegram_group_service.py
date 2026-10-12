"""Telegram group selection use cases."""

from sqlalchemy.orm import Session

from ..repositories.telegram import TelegramRepository


def list_groups(db: Session, available_groups: list[dict[str, object]]) -> list[dict[str, object]]:
    saved = {row.chat_id: row for row in TelegramRepository(db).saved_groups()}
    return [
        {
            **item,
            "enabled": saved[item["chat_id"]].enabled if item["chat_id"] in saved else False,
            "alias": saved[item["chat_id"]].alias if item["chat_id"] in saved else None,
        }
        for item in available_groups
    ]


def save_groups(
    db: Session,
    available_groups: list[dict[str, object]],
    selections: list[dict[str, object]],
) -> dict[str, int]:
    if len({item["chat_id"] for item in selections}) != len(selections):
        raise ValueError("chat_id tidak boleh duplikat")
    available_by_id = {str(item["chat_id"]): item for item in available_groups}
    disabled = TelegramRepository(db).save_group_selections(selections, available_by_id)
    return {"saved": len(selections), "disabled": disabled}
