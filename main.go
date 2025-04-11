package main

import (
	"context" //Пакет для работы с контекстом
	"fmt"
	"log"       //Пакеты для логирования. Записи событий в лог-файл
	"os"        //Пакет для работы с переменными окружения
	"os/signal" //Пакет для работы с сигналами операционной системы
	"syscall"   //Пакет для работы с системными вызовами
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5" //Пакет для работы с Telegram Bot API
	"github.com/joho/godotenv"

	//"github.com/sirupsen/logrus"
	"database/sql" //Подключени SQLite

	_ "github.com/mattn/go-sqlite3"
)

type User struct {
	ID        int64
	Username  string
	FirstName string
	CreatedAt time.Time
}

func initDB() *sql.DB {
	db, err := sql.Open("sqlite3", "bot.db")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
    CREATE TABLE IF NOT EXISTS users (
        id INTEGER PRIMARY KEY,
        username TEXT,
        first_name TEXT,
        created_at DATETIME
    )`)

	if err != nil {
		log.Fatal(err)
	}

	return db
}

func saveUsers(db *sql.DB, user User) error {
	_, err := db.Exec(`
		INSERT INTO users(id, username, first_name, created_at)
		VALUES(?, ?, ?, ?)`,
		user.ID, user.Username, user.FirstName, user.CreatedAt)
	return err
}

func getAllUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query("SELECT id, username, first_name, created_at FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		err := rows.Scan(&u.ID, &u.Username, &u.FirstName, &u.CreatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func isAdmin(userID int64) bool {
	admins := map[int64]bool{
		822725739: true,
	}
	return admins[userID]
}

// Логирование инфо-сообщений
func logInfo(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

// Логирование ошибок
func logError(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

// inline-клавиатура
func createKeyBoard() tgbotapi.InlineKeyboardMarkup { //InlineKeyboardMarkup - создает кнопку с текстом и скрытым значение callback
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow( //Первый ряд кнопок
			tgbotapi.NewInlineKeyboardButtonData("Помощь", "help"),
		),
		tgbotapi.NewInlineKeyboardRow( //Второй ряд кнопок
			tgbotapi.NewInlineKeyboardButtonData("О боте", "about"),
		),
	)
}

func main() {
	if err := godotenv.Load(); err != nil { //Загружает переменные окружения из файла .env
		log.Panic("Ошибка загрузки файла .env") // Если произошла ошибка, то выводит ее в лог
	}

	db := initDB()
	defer db.Close()

	token := os.Getenv("TELEGRAM_BOT_TOKEN") // Получаем токен из переменной окружения
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не задан") // Если токен не установлен, выводим ошибку
	}
	// Создаем экземпляр нового бота с токеном
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	// Удаляем существующий вебхук
	_, err = bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true})
	if err != nil {
		log.Panic("Ошибка удаления вебхука: ", err)
	}

	// Библиотека будет выводить в консоль сырые запросы/ответы к Telegram API.
	bot.Debug = true
	//После успешного подключение выводит в консоль имя бота
	logInfo("Авторизован в акаунте %s", bot.Self.UserName)
	//log.Printf("Авторизован на аакаунте %s", bot.Self.UserName)

	//Создаем контекст, который будет отменен при получении сигнала прерывания
	ctx, cancel := context.WithCancel(context.Background()) //Graceful shutdown
	//WithCancel - создает новый контекст, который будет отменен при вызове cancel()
	// ctx - новый контекст
	// cancel - функция, которая отменяет контекст
	// 	Аналогия:
	// Представим радиоуправляемую игрушку.
	// ctx — сама игрушка, cancel — кнопка выключения на пульте.
	defer cancel() //Отменяем контекст при выходе из функции main
	//defer  - откладывает выполнение функции до тех пор, пока не завершится выполнение функции main

	//Запускаем горутину, которая будет ждать сигнала прерывания
	// это анонимная функция, которая будет выполняться в отдельной горутине
	//Горутина - это легковесный поток, который может выполняться параллельно с другими горутинами
	go func() {
		//Chan - тип для обмена данными между горутинами
		sigs := make(chan os.Signal, 1)                      //Создаем канал для сигналов. Как труба, которая будет принимать сигналы
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM) //Подписываемся на сигналы прерывания для их получения
		//SIGINT - сигнал прерывания (Ctrl+C)
		//SIGTERM - сигнал завершения процесса
		sig := <-sigs                                           //Ждем сигнала. <- Оператор чтения из канала. Программа блокируется до тех пор, пока не получит сигнал
		logInfo("Получен сигнал %s. Завершение работы...", sig) //Выводим в лог полученный сигнал
		cancel()                                                //Отменяем контекст
	}() // Запуск → Создание контекста → Запуск горутины → Ожидание сигнала → Отмена контекста

	u := tgbotapi.NewUpdate(0) //Запрашивает все непрочитанные сообщения
	u.Timeout = 60             //Время ожидания ответа от Telegram API

	updates := bot.GetUpdatesChan(u) //Возвршает канал(как очередь), откуда будет приходить новые сообщения

	// Основной цикл обработки обновлений
	for {
		select {
		// Если контекст завершён, выходим из цикла
		case <-ctx.Done():
			logInfo("Завершаем работу бота...")
			return
		// Обрабатываем новые обновления
		case update := <-updates:
			if update.Message != nil {
				// Это логирование текстовых сообщений - запись в лог информации о том, кто и что написал боту
				// update.Message.From.UserName
				// update - объект с информацией о событии
				// Massege - само сообщение
				// From - информация о пользователе, который отправил сообщение
				// UserName - имя пользователя, который отправил сообщение
				processMessage(db, bot, update.Message)
			}
			if update.CallbackQuery != nil {
				processCallback(bot, update.CallbackQuery)
			}
		}
	}
}

func processMessage(db *sql.DB, bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	log.Printf("[%s] %s", msg.From.UserName, msg.Text)

	if msg.Photo != nil {
		handlePhoto(bot, msg)
		return
	}

	if msg.Document != nil {
		handleDocument(bot, msg)
		return
	}

	var response tgbotapi.MessageConfig
	switch msg.Command() {
	case "start":
		response = handleStart(db, msg)
	case "help":
		response = tgbotapi.NewMessage(msg.Chat.ID, helpText())
	case "about":
		response = tgbotapi.NewMessage(msg.Chat.ID, aboutText())
	case "stats":
		response = handleStats(db, msg)
	case "broadcast":
		response = handleBroadcast(db, bot, msg)
	default:
		response = handleDefault(msg)
	}

	if _, err := bot.Send(response); err != nil {
		logError("Ошибка отправки сообщения: %v", err)
	}
}

func handlePhoto(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	photo := msg.Photo[len(msg.Photo)-1]
	fileURL, _ := bot.GetFileDirectURL(photo.FileID)
	response := tgbotapi.NewMessage(msg.Chat.ID, "Фото сохранено! Вот ссылка"+fileURL)
	bot.Send(response)
}

func handleDocument(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	fileURL, _ := bot.GetFileDirectURL(msg.Document.FileID)
	response := tgbotapi.NewMessage(msg.Chat.ID, "Документ получен! Вот ссылка"+fileURL)
	bot.Send(response)
}

func handleStart(db *sql.DB, msg *tgbotapi.Message) tgbotapi.MessageConfig {
	user := User{
		ID:        msg.From.ID,
		Username:  msg.From.UserName,
		FirstName: msg.From.FirstName,
		CreatedAt: time.Now(),
	}

	if err := saveUsers(db, user); err != nil {
		logError("Ошибка сохранения пользователя: %v", err)
	}

	response := tgbotapi.NewMessage(msg.Chat.ID, "Выберите действия")
	response.ReplyMarkup = createKeyBoard()
	return response
}

func handleStats(db *sql.DB, msg *tgbotapi.Message) tgbotapi.MessageConfig {
	if !isAdmin(msg.From.ID) {
		return tgbotapi.NewMessage(msg.Chat.ID, "У вас нет прав на эту команду")
	}

	users, err := getAllUsers(db)
	if err != nil {
		logError("Ошибка получения пользователей: %v", err)
		return tgbotapi.NewMessage(msg.Chat.ID, "Ошибка получения пользователей")
	}

	return tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("Всего пользователей: %d", len(users)))
}

func handleBroadcast(db *sql.DB, bot *tgbotapi.BotAPI, msg *tgbotapi.Message) tgbotapi.MessageConfig {
	if !isAdmin(msg.From.ID) {
		logError("У вас нет прав на эту команду", msg.From.ID)
		return tgbotapi.NewMessage(msg.Chat.ID, "У вас нет прав на эту команду")
	}

	text := msg.CommandArguments()
	if text == "" {
		return tgbotapi.NewMessage(msg.Chat.ID, "Укажите текст рассылки: /broadcast ваш_текст")
	}

	users, err := getAllUsers(db)
	if err != nil {
		logError("Ошибка получения пользователей: %v", err)
		return tgbotapi.NewMessage(msg.Chat.ID, "Ошибка получения пользователей")
	}

	for _, user := range users {
		m := tgbotapi.NewMessage(user.ID, text)
		if _, err := bot.Send(m); err != nil {
			logError("Ошибка отправки %d: %v", user.ID, err)
		}
	}
	return tgbotapi.NewMessage(msg.Chat.ID, "Рассылка завершена")
}

func handleDefault(msg *tgbotapi.Message) tgbotapi.MessageConfig {
	if msg.IsCommand() {
		return tgbotapi.NewMessage(msg.Chat.ID, "Неизвестная команда")
	}
	return tgbotapi.NewMessage(msg.Chat.ID, "Вы написали: "+msg.Text)
}

func processCallback(bot *tgbotapi.BotAPI, callback *tgbotapi.CallbackQuery) {
	callbackCondig := tgbotapi.NewCallback(callback.ID, "")
	if _, err := bot.Send(callbackCondig); err != nil {
		logError("Ошибка отправки callback: %v", err)
	}

	var msgText string
	switch callback.Data {
	case "help":
		msgText = helpText()
	case "about":
		msgText = aboutText()
	default:
		msgText = "Неизвестная команда"
	}

	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, msgText)
	if _, err := bot.Send(msg); err != nil {
		logError("Ошибка отправки: %v", err)
	}
}

func helpText() string {
	return `Доступные команды:
/start - Начать работу
/help - Помощь
/about - О боте
/stats - Статистика (админы)
/broadcast - Рассылка (админы)`
}

func aboutText() string {
	return `Версия 1.1
Автор: Кирилл Тахмазиди
Telegram: @tahmazidik`
}

// 	   При нажатии Ctrl+C:
//     ОС отправляет процессу сигнал SIGINT
//     Канал sigs получает это значение
//     Горутина разблокируется и вызывает cancel()
// Для большего понимания про Graceful shutdown:
// Аналогия из жизни
// Представь, что ты шеф-повар в ресторане:
//     context — твой кухонный таймер
//     cancel — кнопка остановки таймера
//     SIGINT — звонок из офиса: "Закрываемся через 10 минут!"
//     <-sigs — ты слышишь этот звонок
//     cancel() — ты кричишь: "Всем прекратить готовить!"
//     <-ctx.Done() — повара слышат команду и начинают уборку

// Select - это оператор, который позволяет горутине одновременно ждать выполнения
// нескольких операций с каналами. Это как "перекресток", где программа решает, по какому каналу пойдут данные первыми
