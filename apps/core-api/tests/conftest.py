import os
import sys
from pathlib import Path


sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
os.environ["APP_SECRET_KEY"] = "m1-test-secret-key-at-least-thirty-two-characters"
os.environ["APP_ENV"] = "test"
os.environ["COOKIE_SECURE"] = "false"
os.environ["ADMIN_USERNAME"] = "admin"
os.environ["ADMIN_PASSWORD"] = "M1-Testing-Password-123"
os.environ["DATABASE_URL"] = "sqlite://"
os.environ["EA_API_KEY"] = "m4-test-ea-api-key-32-characters-long"
os.environ.pop("MIGRATION_DATABASE_URL", None)
os.environ.pop("SUPABASE_DEV_PROJECT_REF", None)
os.environ.pop("SUPABASE_PROJECT_REF", None)
