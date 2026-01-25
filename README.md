# warehouse_system

GoEngine web application with full-stack features

## Features

This project includes all GoEngine features:

- ✅ **Middlewares**: Authentication, CORS, Rate Limiting, Logging, Recovery, etc.
- ✅ **Cache**: Redis and In-Memory caching with fallback
- ✅ **Security**: CSRF protection, Password hashing, RBAC, Input sanitization
- ✅ **Storage**: File upload/download with local and cloud support
- ✅ **Observability**: Health checks, Metrics, Request ID tracking
- ✅ **Jobs**: Background jobs with cron scheduling and queue processing
- ✅ **Router**: Advanced routing with v2 features
- ✅ **Server**: Graceful shutdown and production-ready configuration

## Quick Start

Install dependencies:
```bash
go mod tidy
```

Setup database:
```bash
cp .env.example .env
# Edit .env with your database credentials
migrate up
```

Run application:
```bash
make run
```

## Development

Generate CRUD:
```bash
goengine g crud post
```

Generate template routes:
```bash
goengine render
```

Build optimized binary:
```bash
goengine build app
```

Run tests with coverage:
```bash
goengine test -c
```

## Project Structure

- cmd/server/ - Main application entry point
- internal/config/ - Configuration management
- internal/middlewares/ - HTTP middlewares (auth, CORS, rate limiting, etc.)
- internal/cache/ - Caching layer (Redis + in-memory)
- internal/security/ - Security features (CSRF, passwords, RBAC)
- internal/storage/ - File storage handling
- internal/observability/ - Monitoring and health checks
- internal/jobs/ - Background jobs and cron tasks
- internal/router/ - HTTP routing
- internal/server/ - Server lifecycle management
- api/routes/ - Route definitions
- web/ - Templates and static files

## API Documentation

Health check endpoint:
```
GET /health
```

Metrics endpoint:
```
GET /metrics
```

## Environment Variables

See .env.example for all available configuration options.

## License

MIT
