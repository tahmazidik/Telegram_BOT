package main

import (
	"context"   //Пакет для работы с контекстом
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
	token := os.Getenv("TELEGRAM_BOT_TOKEN") // Получаем токен из переменной окружения
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не задан") // Если токен не установлен, выводим ошибку
	}
	// Создаем экземпляр нового бота с токеном
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	db := initDB()
	defer db.Close()

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
				log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

				//Обработка изображений
				if update.Message.Photo != nil {
					//Берем последнее самое качественное фото из массива
					photo := update.Message.Photo[len(update.Message.Photo)-1]
					fileURL, err := bot.GetFileDirectURL(photo.FileID)
					if err != nil {
						logError("Ошибка получения файла: %v", err)
						msg = tgbotapi.NewMessage(chatID, "Не удалось обработать фото")
					} else {
						msg = tgbotapi.NewMessage(chatID, "Фото сохранено! Вот ссылка: "+fileURL)
						//TODO: здесь можно сохрнаить файл в БД
					}
					bot.Send(msg)
					continue
				}
				//Обработка документов
				if update.Message.Document != nil {
					fileURL.err := bot.GetFileDirectURL(update.Message.Document.FileID)
					if err != nil {
						logError("Ошибка получения документа: %v", err)
						msg = tgbotapi.NewMessage(chatID, "Ошибка загрузки файла")
					} else {
						msg = tgbotapi.NewMessage(chatID, "Документ получен! Вот ссылка "+fileURL)
					}
					bot.Send(msg)
					continue
				}

				var msg tgbotapi.MessageConfig
				switch update.Message.Command() {
				case "start":
					//Сохранение пользователя
					user := User{
						ID:        update.Message.From.ID,
						UserName:  update.Message.From.UserName,
						FirstName: update.Message.From.FirstName,
						CreatedAt: time.Now(),
					}
					msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Выберите действия")
					msg.ReplyMarkup = createKeyBoard()
				case "help":
					msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Вот список доступных команд:\n/start - начать взаимодействие\n/help - получить помощь\n/about - узнать версию бота, его создателя и связь с ним\n/stats - показать статистику бота.")
				case "about":
					msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Версия 1.0.0\nАвтор: Кирилл Тахмазиди\nTelegram: @tahmazidik")
				case "stats":
					if isAdmin(update.Message.From.ID) {
						count := getTotalUsers(db)
						msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Всего пользователей: %d", count)
					}
				case "broadcast":
					if isAdmin(update.Message.From.ID) {
						//Получаем текст рассылки
						args := update.Message.CommandArguments()
						if args == "" {
							msg = tgbotapi.NewMessage(charID, "Укажите текст рассылки: /broadcast Привет всем!")
							break
						}

						//Получаем всех пользователей из БД
						users, err := getAllUsers()
						if err != nil {
							logError("Ошибка получение пользователей: %v", err)
							break
						}

						//Отправляем каждому
						for _, user := range users {
							broadcastMsg := tgbotapi.NewMessage(user.ID, args)
							if _, err := bot.Send(broadcastMsg); err != nil {
								logError("Ошибка отправка пользователю %d %v", user.ID, err)
							}
						}
						msg = tgbotapi.NewMessage(charID, "Рассылка завершена!")
					} else {
						msg = tgbotapi.NewMessage(chatID, "У вас нет прав на эту команду")
					}
				default:
					if !update.Message.IsCommand() {
						msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Вы написали: "+update.Message.Text)
					} else {
						msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Я не знаю такой команды. Напиши /help, чтобы получить список доступных команд.")
					}
				}

				if _, err := bot.Send(msg); err != nil {
					logError("Ошибка отправки сообщения: %v", err)
				}
			}

			// Обработка нажатий на inline-кнопки
			if update.CallbackQuery != nil {
				callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
				if _, err := bot.Send(callback); err != nil {
					logError("Ошибка отправки callback: %v", err)
				}

				var msgText string
				switch update.CallbackQuery.Data {
				case "help":
					msgText = "Используй команды /start и /about"
				case "about":
					msgText = "Версия 1.0.0\nАвтор: Кирилл Тахмазиди\nTelegram: @tahmazidik"
				default:
					msgText = "Неизвестная команда кнопки"
				}
				msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, msgText)
				if _, err := bot.Send(msg); err != nil {
					logError("Ошибка отправки сообщения: %v", err)
				}
			}
		}
	}
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
