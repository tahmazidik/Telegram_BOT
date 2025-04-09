package main

import (
	"context"   //Пакет для работы с контекстом
	"log"       //Пакеты для логирования. Записи событий в лог-файл
	"os"        //Пакет для работы с переменными окружения
	"os/signal" //Пакет для работы с сигналами операционной системы
	"syscall"   //Пакет для работы с системными вызовами

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5" //Пакет для работы с Telegram Bot API
	"github.com/joho/godotenv"
)

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
				var msg tgbotapi.MessageConfig
				switch update.Message.Command() {
				case "start":
					msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Выберите действия")
					msg.ReplyMarkup = createKeyBoard()
				case "help":
					msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Вот список доступных команд:\n/start - начать взаимодействие\n/help - получить помощь.")
				case "about":
					msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Версия 1.0.0\nАвтор: Кирилл Тахмазиди\nTelegram: @tahmazidik")
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
