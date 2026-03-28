# stashfor.me - Architecture Documentation

A link sharing site that allows users to save links, organize them into lists, and share them via SMS.

## Tech Stack

- **Go 1.25.0** - Backend language
- **Echo v4** - Web framework
- **Templ** - Type-safe Go templating (compiles to Go)
- **PostgreSQL** - Database
- **WebAuthn/Passkeys** - Passwordless authentication
- **Goose** - Database migrations

## Directory Structure

```
stashforme/
├── cmd/server/main.go        # Application entry point
├── internal/
│   ├── auth/                 # Authentication services
│   │   ├── user.go          # User model & UserStore
│   │   ├── session.go       # Session management
│   │   ├── otp.go           # OTP verification service
│   │   ├── passkey.go       # WebAuthn/passkey service
│   │   ├── webauthn_data.go # PasskeyRegisterData helper
│   │   └── validate.go      # Phone number validation
│   ├── database/            # Database connection & migrations
│   ├── handlers/            # HTTP request handlers
│   │   ├── auth.go          # Auth endpoints
│   │   ├── account.go       # Account endpoints
│   │   ├── stash.go         # Stash/list/URL management endpoints
│   │   ├── response.go      # HATEOAS response helpers
│   │   ├── cookies.go       # Cookie management helpers
│   │   ├── context.go       # Context helpers (RequireUser)
│   │   ├── urls.go          # URL/path constants
│   │   └── handlers.go      # Handler base & Render helper
│   ├── middleware/auth.go   # Session validation middleware
│   ├── sms/                 # SMS provider abstraction
│   └── views/               # Templ templates
│       ├── layout.templ     # Main layout with header/nav/footer
│       ├── home.templ       # Landing page
│       ├── login.templ      # Login form
│       ├── verify.templ     # OTP verification
│       ├── passkey_setup.templ  # Passkey registration
│       ├── account.templ    # Account settings
│       ├── stash.templ      # Main stash page
│       ├── stash_partials.templ # List/URL components
│       └── partials.templ   # Shared components (ProfileCard)
├── static/                  # CSS, JavaScript (app.js, style.css)
└── Makefile                # Build commands
```

## Architecture Patterns

### MVC + Service Layer
- **Models**: User, Session, Passkey (`internal/auth/`)
- **Views**: Templ templates (server-side rendering)
- **Controllers**: Echo handlers
- **Services**: Business logic (OTPService, PasskeyService, UserStore, SessionStore)

### Repository/Store Pattern
Services manage data access:
- `UserStore` - user CRUD
- `SessionStore` - session management
- `PasskeyService` - WebAuthn credentials
- `ListStore` - list CRUD operations
- `URLStore` - URL metadata and deduplication
- `ListURLStore` - list-URL relationships

### HATEOAS API Responses
JSON responses include `_links` for navigation:
```json
{
  "data": {...},
  "_links": { "redirect": { "href": "/path" } }
}
```

### Dependency Injection
Services receive dependencies via constructors. Main.go orchestrates initialization.

## Authentication Flow

Three auth methods:
1. **OTP/SMS** - Phone number + 6-digit code (10 min expiry, max 3 attempts)
2. **Passkey Registration** - After OTP, prompted to add passkey
3. **Passkey Login** - Direct WebAuthn authentication

Sessions: 32-byte random tokens, SHA256 hashed in DB, 30-day expiry.

## Database Schema

Core tables:
- `users` - id, phone_number (E.164), display_name, timestamps
- `sessions` - token_hash, user_id, expires_at, last_active_at
- `verification_codes` - OTP codes with expiry & attempt tracking
- `passkeys` - WebAuthn credentials per user
- `lists`, `urls`, `list_urls` - Link organization with full CRUD support

## Naming Conventions

| Type | Pattern | Example |
|------|---------|---------|
| Handlers | Action verbs | `Login()`, `SendCode()`, `VerifyCode()` |
| Store methods | Query-oriented | `FindByPhone()`, `FindOrCreate()` |
| Errors | `Err<Issue>` | `ErrUserNotFound`, `ErrOTPExpired` |
| Templates | `<page>.templ` | `login.templ`, `account.templ` |
| Migrations | Sequential | `00001_create_users.sql` |

## Key Routes

**Public:**
- `GET /` - Landing (redirects to `/my/stash` if logged in)
- `GET /login` - Login form
- `GET /verify` - OTP verification page
- `GET /passkey/setup` - Passkey registration page
- `POST /auth/send-code` - Initiate OTP
- `POST /auth/verify-code` - Verify OTP
- `POST /auth/passkey/register` - Complete passkey registration
- `POST /auth/passkey/login` - Start passkey auth
- `POST /auth/passkey/login/finish` - Complete passkey auth
- `POST /auth/skip-passkey` - Skip passkey setup
- `GET /api/ping` - Health check

**Protected (require auth):**
- `GET /my` - Redirects to `/my/stash`
- `GET /my/stash` - Link stash
- `GET /my/stash/new` - New list form
- `GET /my/stash/lists/:id` - List detail view
- `POST /my/stash/lists` - Create list
- `PUT /my/stash/lists/:id` - Update list
- `DELETE /my/stash/lists/:id` - Delete list
- `GET /my/stash/:id/new` - New URL form
- `POST /my/stash/lists/:id/urls` - Add URL to list
- `GET /my/stash/urls/:id` - URL item view
- `GET /my/stash/urls/:id/edit` - Edit URL notes form
- `PUT /my/stash/urls/:id` - Update URL notes
- `DELETE /my/stash/urls/:id` - Remove URL from list
- `GET /my/account` - Account settings
- `POST /my/account/passkeys/register` - Add new passkey
- `DELETE /my/account/passkeys/:id` - Remove passkey (supports `_method` override)
- `POST /auth/logout` - Logout

## Development Commands

```bash
make dev              # Run with hot reload
make generate         # Compile Templ templates
make build            # Build executable
make test             # Run tests
make migrate          # Run migrations
make migrate-down     # Rollback last migration
make migrate-create NAME=foo  # New migration
```

## Environment Variables

```
DATABASE_URL          # PostgreSQL connection string
SMS_PROVIDER          # mock | twilio
TWILIO_*              # Twilio credentials (if using)
WEBAUTHN_RP_ID        # Domain (e.g., localhost)
WEBAUTHN_RP_ORIGIN    # Full origin URL
WEBAUTHN_RP_NAME      # Display name
PORT                  # Server port (default 8080)
```

## Code Principles

- **Separation of Concerns**: auth/ has no HTTP knowledge, handlers/ delegates business logic
- **Interface-Based**: SMS provider is swappable (mock for dev, Twilio for prod)
- **Semantic HTML**: Proper elements (`<dl>`, `<section>`), ARIA attributes, skip-to-main link
- **Accessibility**: `aria-invalid`/`aria-errormessage` on form errors, `aria-live` for alerts
- **Security**: HttpOnly cookies, SameSite, rate limiting, OTP expiry, method override for DELETE

## Gotchas

- Phone numbers must be E.164 format (+14155551234)
- Templ files generate `*_templ.go` files - don't edit generated files
- WebAuthn cookies use SameSite=Strict (stricter than session cookies)
- WebAuthn User.ID can be `[]byte` or `protocol.URLEncodedBase64` - use type switch
- OTP codes limited to 10/hour per phone number
- Session tokens stored as SHA256 hashes, not plaintext
- DELETE forms use `_method=DELETE` hidden field with MethodOverride middleware
- WebAuthn options must return raw JSON (not HATEOAS wrapped) for JS WebAuthn API
