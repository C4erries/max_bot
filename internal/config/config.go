package config

// Config содержит все параметры, необходимые для запуска бота.
type Config struct {
	Bot     BotConfig
	HTTP    HTTPConfig
	Logger  LoggerConfig
	Backend BackendConfig
}

// BotConfig хранит токен и базовые настройки для Max.
type BotConfig struct {
	Token string `env:"BOT_TOKEN,required"`
}

// HTTPConfig задаёт адрес вспомогательного HTTP-сервера.
type HTTPConfig struct {
	Address      string `env:"HTTP_ADDRESS" envDefault:":8080"`
	BackendToken string `env:"HTTP_BACKEND_TOKEN"`
}

// LoggerConfig описывает уровень логирования.
type LoggerConfig struct {
	Level string `env:"LOG_LEVEL" envDefault:"info"`
}

// BackendConfig описывает параметры взаимодействия с backend API.
type BackendConfig struct {
	APIBaseURL string `env:"BACKEND_API_BASE_URL" envDefault:""`
}
