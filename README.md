# StashForMe

Save links, organize them into lists, and share them via SMS.

## Features

- **Passwordless Authentication** - Sign in with SMS verification codes or passkeys (Face ID, Touch ID, device PIN)
- **Link Stash** - Save and organize your favorite links
- **Lists** - Group links into shareable collections
- **SMS Sharing** - Send links to friends via text message

## Tech Stack

- **Go 1.25** with [Echo](https://echo.labstack.com/) web framework
- **[Templ](https://templ.guide/)** for type-safe HTML templates
- **PostgreSQL** for data persistence
- **[WebAuthn](https://webauthn.io/)** for passkey authentication
- **[Goose](https://pressly.github.io/goose/)** for database migrations

## Getting Started

### Prerequisites

- Go 1.25+
- PostgreSQL
- Make

### Install Tools

```bash
make install-tools
```

This installs:
- `templ` - Template compiler
- `air` - Hot reload for development
- `goose` - Database migrations

### Configure Environment

Create a `.env` file:

```bash
DATABASE_URL=postgres://user:pass@localhost:5432/stashforme?sslmode=disable

# SMS Provider (mock for development, twilio for production)
SMS_PROVIDER=mock

# For Twilio (production)
# TWILIO_ACCOUNT_SID=your_sid
# TWILIO_AUTH_TOKEN=your_token
# TWILIO_FROM_NUMBER=+15551234567

# WebAuthn (adjust for your domain)
WEBAUTHN_RP_ID=localhost
WEBAUTHN_RP_ORIGIN=http://localhost:8080
WEBAUTHN_RP_NAME=StashForMe

PORT=8080
```

### Setup Database

```bash
createdb stashforme
make migrate
```

### Run

```bash
# Development with hot reload
make dev

# Or build and run
make build
./bin/server
```

Visit http://localhost:8080

## Makefile Commands

### Development

| Command | Description |
|---------|-------------|
| `make dev` | Run with hot reload (uses [air](https://github.com/air-verse/air)) |
| `make run` | Build and run the server |
| `make build` | Build executable to `bin/server` |
| `make generate` | Compile Templ templates to Go |
| `make test` | Run tests |
| `make clean` | Remove build artifacts and generated files |
| `make tidy` | Run `go mod tidy` |

### Database Migrations

| Command | Description |
|---------|-------------|
| `make migrate` | Run all pending migrations |
| `make migrate-down` | Rollback the last migration |
| `make migrate-create NAME=foo` | Create a new migration file |
| `make migrate-status` | Show current migration status |

### Setup

| Command | Description |
|---------|-------------|
| `make install-tools` | Install templ, air, and goose |

## Project Structure

```
stashforme/
├── cmd/server/         # Application entry point
├── internal/
│   ├── auth/           # Authentication (users, sessions, OTP, passkeys)
│   ├── database/       # Database connection & migrations
│   ├── handlers/       # HTTP request handlers
│   ├── middleware/     # Auth middleware
│   ├── sms/            # SMS provider abstraction
│   └── views/          # Templ templates
├── static/             # CSS and JavaScript
└── Makefile
```

## License

MIT
