function Clear-AlgentraApplicationEnvironment {
    param([string[]] $Preserve = @())

    $applicationEnvironmentNames = @(
        'APP_ENV', 'ALGENTRA_ENV_FILE', 'PROJECT_ROOT', 'APP_SECRET_KEY', 'ADMIN_USERNAME', 'ADMIN_PASSWORD',
        'DATABASE_URL', 'DATABASE_ADMIN_URL', 'ALGENTRA_API_PASSWORD', 'MIGRATION_DATABASE_URL',
        'SUPABASE_DEV_PROJECT_REF', 'SUPABASE_PROJECT_REF', 'EA_API_KEY', 'COPIER_WORKER_API_KEY',
        'MASTER_PUBLISHER_TOKEN', 'HTTP_ADDR', 'WEBSOCKET_ALLOWED_ORIGINS', 'COOKIE_SECURE',
        'CONTACT_PERSON_NAME', 'CONTACT_WHATSAPP', 'CONTACT_EMAIL', 'CONTACT_INSTAGRAM',
        'CONTACT_TELEGRAM', 'CONTACT_TELEGRAM_ADMIN', 'CONTACT_TIKTOK', 'CONTACT_WEBSITE',
        'CONTACT_FACEBOOK', 'CONTACT_LINKEDIN', 'CONTACT_X', 'CONTACT_YOUTUBE', 'PUBLIC_MUSIC_URL',
        'GEMINI_API_KEY', 'GEMINI_MODEL', 'PAYMENT_BANK_NAME', 'PAYMENT_NUMBER', 'PAYMENT_NAME',
        'MIDTRANS_SERVER_KEY', 'MIDTRANS_MERCHANT_ID', 'MIDTRANS_IS_PRODUCTION', 'MIDTRANS_NOTIFICATION_URL',
        'TELEGRAM_API_ID', 'TELEGRAM_API_HASH', 'TELEGRAM_BOT_TOKEN', 'TELEGRAM_ADMIN_CHAT_ID',
        'EMAIL_SMTP_HOST', 'EMAIL_SMTP_PORT', 'EMAIL_SMTP_USERNAME', 'EMAIL_SMTP_PASSWORD',
        'EMAIL_FROM', 'EMAIL_SMTP_STARTTLS', 'MANAGEMENT_FEE_IDR'
    )
    for ($index = 1; $index -le 5; $index++) {
        $applicationEnvironmentNames += @("VPS${index}_NAME", "VPS${index}_VCPU", "VPS${index}_RAM", "VPS${index}_DISK", "VPS${index}_PRICE_IDR")
    }

    foreach ($name in $applicationEnvironmentNames) {
        if ($Preserve -notcontains $name) {
            [Environment]::SetEnvironmentVariable($name, $null, 'Process')
        }
    }
}
