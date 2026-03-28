package main

import (
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"stashforme/internal/auth"
	"stashforme/internal/database"
	"stashforme/internal/handlers"
	mw "stashforme/internal/middleware"
	"stashforme/internal/sms"
	"stashforme/internal/stash"
)

func main() {
	// Load .env file (ignore error if not present)
	_ = godotenv.Load()

	// Connect to database
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}
	db, err := database.ConnectURL(databaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Run migrations
	if err := database.Migrate(db, "internal/database/migrations"); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}

	// Initialize SMS provider
	var smsProvider sms.Provider
	switch getEnv("SMS_PROVIDER", "mock") {
	case "twilio":
		smsProvider = sms.NewTwilioProvider(sms.TwilioConfig{
			AccountSID: os.Getenv("TWILIO_ACCOUNT_SID"),
			AuthToken:  os.Getenv("TWILIO_AUTH_TOKEN"),
			FromNumber: os.Getenv("TWILIO_FROM_NUMBER"),
		})
		log.Println("Using Twilio SMS provider")
	default:
		smsProvider = sms.NewMockProvider()
		log.Println("Using Mock SMS provider (codes will be logged to console)")
	}

	// Initialize stores
	userStore := auth.NewUserStore(db)
	sessionStore := auth.NewSessionStore(db)
	listStore := stash.NewListStore(db)
	urlStore := stash.NewURLStore(db)
	listURLStore := stash.NewListURLStore(db)

	// Initialize services
	otpService := auth.NewOTPService(db, smsProvider)

	// Initialize passkey service
	passkeyConfig := auth.PasskeyConfig{
		RPID:          getEnv("WEBAUTHN_RP_ID", "localhost"),
		RPOrigin:      getEnv("WEBAUTHN_RP_ORIGIN", "http://localhost:8080"),
		RPDisplayName: getEnv("WEBAUTHN_RP_NAME", "stashfor.me"),
	}
	passkeyService, err := auth.NewPasskeyService(db, userStore, passkeyConfig)
	if err != nil {
		log.Fatal("Failed to create passkey service:", err)
	}

	// Initialize Echo
	e := echo.New()

	// Middleware
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		LogMethod:   true,
		LogLatency:  true,
		HandleError: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			log.Printf("%s %s %d %v", v.Method, v.URI, v.Status, v.Latency)
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Pre(middleware.MethodOverrideWithConfig(middleware.MethodOverrideConfig{
		Getter: middleware.MethodFromForm("_method"),
	}))
	e.Use(mw.Auth(sessionStore, userStore))

	// Static files
	e.Static("/static", "static")

	// Initialize handlers
	h := handlers.New()
	authHandler := handlers.NewAuthHandler(otpService, sessionStore, userStore, passkeyService)
	accountHandler := handlers.NewAccountHandler(userStore, passkeyService)
	stashHandler := handlers.NewStashHandler(listStore, urlStore, listURLStore)

	// Public routes
	e.GET("/", h.Home)
	e.GET("/api/ping", h.Ping)

	// Protected routes
	e.GET("/my", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/my/stash")
	})
	e.GET("/my/account", accountHandler.Account)
	e.POST("/my/account/passkeys/register", accountHandler.RegisterPasskey)
	e.DELETE("/my/account/passkeys/:id", accountHandler.DeletePasskey)

	// Stash routes
	e.GET("/my/stash", stashHandler.Stash)
	e.GET("/my/stash/new", stashHandler.NewListPage)
	e.GET("/my/stash/lists/:id", stashHandler.ListDetail)
	e.POST("/my/stash/lists", stashHandler.CreateList)
	e.PUT("/my/stash/lists/:id", stashHandler.UpdateList)
	e.DELETE("/my/stash/lists/:id", stashHandler.DeleteList)
	e.GET("/my/stash/:id/new", stashHandler.NewURLPage)
	e.POST("/my/stash/lists/:id/urls", stashHandler.AddURL)
	e.GET("/my/stash/urls/:id", stashHandler.URLItem)
	e.GET("/my/stash/urls/:id/edit", stashHandler.EditURLNotesForm)
	e.PUT("/my/stash/urls/:id", stashHandler.UpdateURLNotes)
	e.DELETE("/my/stash/urls/:id", stashHandler.RemoveURL)

	// Auth routes
	e.GET("/login", authHandler.Login)
	e.POST("/auth/send-code", authHandler.SendCode)
	e.GET("/verify", authHandler.VerifyPage)
	e.POST("/auth/verify-code", authHandler.VerifyCode)
	e.GET("/passkey/setup", authHandler.PasskeySetupPage)
	e.POST("/auth/passkey/register", authHandler.PasskeyRegister)
	e.POST("/auth/passkey/login", authHandler.PasskeyLogin)
	e.POST("/auth/passkey/login/finish", authHandler.PasskeyLoginFinish)
	e.POST("/auth/skip-passkey", authHandler.SkipPasskey)
	e.POST("/auth/logout", authHandler.Logout)

	// Start server
	port := getEnv("PORT", "8080")
	log.Printf("Starting server on :%s", port)
	e.Logger.Fatal(e.Start(":" + port))
}

// getEnv returns the environment variable value or a default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
