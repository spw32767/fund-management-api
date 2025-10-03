# Backend Deployment Guide

This guide documents how to deploy and maintain the Go (Gin + GORM) API for the fund management platform.

## 1. Prerequisites

Install the required system packages on the server:

- `git`
- Build essentials (`build-essential`, `libssl-dev` on Debian/Ubuntu)
- LibreOffice (for publication reward PDF rendering)
- Go **1.21+** (or the version declared in `go.mod`)
- MariaDB/MySQL server and client libraries (for example `mariadb-server` and `libmariadb-dev`)

```bash
# Debian / Ubuntu example
sudo apt update
sudo apt install -y git build-essential libssl-dev libreoffice mariadb-server libmariadb-dev

# Install Go 1.21+
wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

Create a database and user for the application before running the API.

## 2. Initial Deployment

```bash
# Clone the repository
cd /opt
sudo git clone https://<your-git-host>/fund-management-unify.git
sudo chown -R $USER:$USER fund-management-unify

# Enter the backend project
cd fund-management-unify/fund-management-api

# Sync dependencies
go mod tidy
```

## 3. Environment Configuration

Copy the sample environment file and adjust values for your environment:

```bash
cp .env.example .env
```

Set at minimum the following variables (see `.env.example` for the full list):

- `DB_HOST`, `DB_PORT`, `DB_DATABASE`, `DB_USERNAME`, `DB_PASSWORD`
- `SERVER_PORT`
- `GIN_MODE=release` in production
- `JWT_SECRET`, `JWT_EXPIRE_HOURS`, `REFRESH_TOKEN_EXPIRE_HOURS`
- `UPLOAD_PATH` (directory for uploaded files)
- Email/notification settings if used (`SMTP_*`, `APP_BASE_URL`)

Ensure the directories referenced by `UPLOAD_PATH` and `LOG_FILE` exist and are writable by the service user.

## 4. Database Migrations

Import the provided SQL schema that matches your target version (for example `fund_cpkku_v42.sql`).

```bash
mysql -u <db_user> -p<db_password> <db_name> < ../fund_cpkku_v42.sql
```

Run migrations again whenever a new SQL dump or migration script is added to the repository.

## 5. Build and Run

```bash
# Build the API binary
go build -o fund-api ./cmd/api

# Run manually (for testing)
./fund-api
```

For persistent deployments, run the binary with a process manager. Example `systemd` service (`/etc/systemd/system/fund-api.service`):

```ini
[Unit]
Description=Fund Management API
After=network.target mariadb.service

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/fund-management-unify/fund-management-api
EnvironmentFile=/opt/fund-management-unify/fund-management-api/.env
ExecStart=/opt/fund-management-unify/fund-management-api/fund-api
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Reload systemd and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now fund-api.service
```

## 6. Updating an Existing Deployment

```bash
cd /opt/fund-management-unify/fund-management-api

git pull

go mod tidy

go build -o fund-api ./cmd/api
sudo systemctl restart fund-api.service  # or restart your process manager
```

Check the release notes or diff for new environment variables or database migrations before restarting the service.