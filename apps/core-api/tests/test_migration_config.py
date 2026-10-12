import pytest

from app import migration_config


def test_process_environment_cannot_relabel_production_env_file_as_development(tmp_path, monkeypatch):
    (tmp_path / ".env.local").write_text("APP_ENV=production\n", encoding="utf-8")
    monkeypatch.setattr(migration_config, "PROJECT_ROOT", tmp_path)
    monkeypatch.setenv("APP_ENV", "development")

    with pytest.raises(RuntimeError, match="APP_ENV=development"):
        migration_config.load_development_database_config()
