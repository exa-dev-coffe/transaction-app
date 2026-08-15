# Transaction Service (transaction-service)

Transaction Service is the core orchestration service built with **Go** and **Fiber**. It handles user checkout flows, interacts with multiple other services (Wallet, Master Data), and publishes events for async processing.

## 🚀 Technologies

*   **Language**: Go 1.25
*   **Framework**: Fiber v2
*   **Database**: PostgreSQL
*   **SQL Toolkit**: `sqlx`
*   **Message Broker**: RabbitMQ (`amqp091-go`)
*   **Observability**: OpenTelemetry (`otelsql`, HTTP client tracing)
*   **Logging**: `log/slog`

## 📦 Features

*   **Order Creation**: Orchestrates cart checkout by fetching menu data and communicating with Wallet Service for payments.
*   **HMAC Security**: Generates and signs payloads using HMAC to securely call internal APIs (Account, Wallet, Master Data).
*   **Transaction History**: Provides detailed transaction and order history to the user.
*   **Event Publisher**: Dispatches events (e.g., `menu.set_rating`) to RabbitMQ for workers to process.

## 🛠️ Prerequisites

*   Go 1.25+
*   PostgreSQL
*   RabbitMQ

## ⚙️ Environment Variables

Copy `.env.example` to `.env`:

```bash
cp .env.example .env
```

## 🚀 How to Run

1.  **Download Dependencies:**
    ```bash
    go mod download
    ```

2.  **Run Locally:**
    ```bash
    go run main.go
    ```
