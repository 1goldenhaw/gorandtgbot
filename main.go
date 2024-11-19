/*Code of tg-bot on golang, that make inline button to get random post from channel, where the bot is adminю
To start working you should add env for Token and add channelID of the channel*/

package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	tb "gopkg.in/telebot.v4"
)

var ChatIDWithUser int64

// ChannelCache - struct to save cache of MessageID
type ChannelCache struct {
	sync.RWMutex
	Data map[int64][]int // channelID -> list of messageID
}

// Global Cache
var messageCache = &ChannelCache{
	Data: make(map[int64][]int),
}

// startCacheUpdater - запуск обновления кэша сообщений
func startCacheUpdater(bot *tb.Bot, channelIDs []int64, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		for _, channelID := range channelIDs {
			go func(chID int64) {
				log.Printf("Обновление кэша для канала ID=%d", chID)
				updateMessageCache(bot, chID)
			}(channelID)
		}
	}
}

// Получение максимального messageID через тестовое сообщение
func getMaxMessageID(bot *tb.Bot, channelID int64) (int, error) {
	// Отправляем тестовое сообщение
	testMessage, err := bot.Send(&tb.Chat{ID: channelID}, "Тестовое сообщение", &tb.SendOptions{
		DisableNotification: true, // Без уведомлений
	})
	if err != nil {
		return 0, err
	}

	// Логируем ID тестового сообщения
	log.Printf("Тестовое сообщение отправлено, ID=%d", testMessage.ID)

	// Удаляем тестовое сообщение
	if err := bot.Delete(testMessage); err != nil {
		log.Printf("Не удалось удалить тестовое сообщение ID=%d: %v", testMessage.ID, err)
	}

	// Возвращаем ID тестового сообщения как максимальный
	return testMessage.ID, nil
}

// Обновление кэша сообщений для канала
func updateMessageCache(bot *tb.Bot, channelID int64) {
	log.Printf("Начинаем обновление кэша для канала ID=%d", channelID)

	// Получаем максимальный messageID
	maxMessageID, err := getMaxMessageID(bot, channelID)
	if err != nil {
		log.Printf("Ошибка получения максимального messageID для канала ID=%d: %v", channelID, err)
		return
	}

	log.Printf("Максимальный messageID для канала ID=%d: %d", channelID, maxMessageID)

	var messageIDs []int
	for i := 1; i <= maxMessageID; i++ {
		msg := &tb.Message{
			Chat: &tb.Chat{ID: channelID},
			ID:   i,
		}

		// Проверяем сообщение
		_, err := bot.Forward(bot.Me, msg)
		if err == nil {
			// Сообщение успешно проверено и существует
			messageIDs = append(messageIDs, i)
			continue
		}

		// Обработка ошибок
		errMsg := err.Error()
		switch {
		case errMsg == "telegram: Forbidden: bots can't send messages to bots (403)":
			// Считаем сообщение существующим
			messageIDs = append(messageIDs, i)
			log.Printf("Успех: Сообщение с ID=%d существует в канале ID=%d, запоминаем...\n", i, channelID)
		case errMsg == "Bad Request: message to forward not found":
			log.Printf("Ошибка: Сообщение ID=%d не найдено в канале ID=%d\n", i, channelID)
		default:
			log.Printf("Неизвестная ошибка при обработке messageID=%d для канала ID=%d: %v\n", i, channelID, err)
		}
	}

	// Обновляем кэш
	messageCache.Lock()
	messageCache.Data[channelID] = messageIDs
	messageCache.Unlock()
	log.Printf("Кэш обновлён для канала ID=%d: %d сообщений", channelID, len(messageIDs))
	str := fmt.Sprintf("Кэш обновлён для канала ID=%d: %d сообщений", channelID, len(messageIDs))
	_, err = bot.Send(tb.ChatID(ChatIDWithUser), str)
	if err != nil {
		log.Printf("Ошибка отправки сообщения пользователю chatID=%d: %v", ChatIDWithUser, err)
	}
}

func main() {
	// Настройки бота
	pref := tb.Settings{
		Token:  os.Getenv("TELEGRAM_BOT_TOKEN"),
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := tb.NewBot(pref)
	if err != nil {
		log.Fatalf("Ошибка инициализации бота: %v", err)
	}

	// Список каналов, где бот является администратором
	var adminChannels []int64
	// adminChannels = append(adminChannels, -1001744445555) // you can to hardcode channelID of all of your channel unless I don't provide opportinity to do this automaticly or from bot interface.

	// Обработка команды /start
	bot.Handle("/start", func(c tb.Context) error {
		ChatIDWithUser = c.Chat().ID
		return c.Send("Нажми на кнопку - получишь результат!", &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{
					tb.InlineButton{
						Unique: "random_post",
						Text:   "Рандомный пост",
					},
					tb.InlineButton{
						Unique: "update",
						Text:   "Обновить",
					},
				},
			},
		})
	})

	// Обработка добавления в канал
	bot.Handle(tb.OnAddedToGroup, func(c tb.Context) error {
		chat := c.Chat()
		adminChannels = append(adminChannels, chat.ID)
		go updateMessageCache(bot, chat.ID) // Обновляем кэш для нового канала
		log.Printf("Бот добавлен в канал: ID=%d, Title=%s", chat.ID, chat.Title)
		return c.Send("Спасибо за добавление в канал!")
	})


	bot.Handle(&tb.InlineButton{Unique: "random_post"}, func(c tb.Context) error {
		if len(adminChannels) == 0 {
			return c.Send("Я пока не являюсь администратором ни одного канала. Добавьте меня в канал и попробуйте снова.")
		}

		// Выбираем случайный канал
		randomChannelID := adminChannels[rand.Intn(len(adminChannels))]

		// Получаем кэш сообщений для выбранного канала
		messageCache.RLock()
		messageIDs := messageCache.Data[randomChannelID]
		messageCache.RUnlock()

		if len(messageIDs) == 0 {
			return c.Send("Для выбранного канала нет доступных сообщений. Попробуйте позже.")
		}

		// Выбираем случайное сообщение
		randomMessageID := messageIDs[rand.Intn(len(messageIDs))]

		// Пересылаем сообщение пользователю
		msg := &tb.Message{
			Chat: &tb.Chat{ID: randomChannelID},
			ID:   randomMessageID,
		}
		originalMessage, err := bot.Copy(c.Sender(), msg)
		if err != nil {
			log.Printf("Ошибка копирования сообщения ID=%d из канала ID=%d: %v", randomMessageID, randomChannelID, err)
			return c.Send("Не удалось отправить сообщение. Попробуйте снова.")
		}

		log.Printf("Сообщение отправлено пользователю: %+v", originalMessage)
		return nil
	})

	// Обработчик кнопки "update"
	bot.Handle(&tb.InlineButton{Unique: "update"}, func(c tb.Context) error {
		if len(messageCache.Data[adminChannels[0]]) == 0 {
			go updateMessageCache(bot, adminChannels[0])
			return c.Send("Пошло обновление!")
		} else {
			var st string
			st = "База не пустая, сообщений где-то... " + string(rune(len(messageCache.Data[adminChannels[0]])))
			return c.Send(st)
		}
	})

	// Запуск обновления кэша
	go startCacheUpdater(bot, adminChannels, 300*time.Minute)

	// Запуск бота
	log.Println("Бот запущен!")
	fmt.Println(adminChannels)
	bot.Start()
}
