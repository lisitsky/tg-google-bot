package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/pkg/errors"
)

const GoogleSearchURL = "https://www.google.ru/search?q=&oe=utf-8&ie=utf-8"

var (
	parsedURL *url.URL

	botapi Telegramer //*tgbotapi.BotAPI
)

func main() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)

	// set exit signal listener
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	var err error
	parsedURL, err = url.Parse(GoogleSearchURL)
	if err != nil {
		log.Fatal(fmt.Sprintf("cannot parse url template: %v error %v", GoogleSearchURL, err))
	}

	updateChan, err := startTelegram()
	if err != nil {
		log.Fatalf("Cannot start: %v", err)
	}
	go telegramUpdater(updateChan)
	log.Printf("Started")

	// wait for exit signal
	s := <-c
	log.Printf("got exit signal: %v", s)
}

func telegramUpdater(updateChan tgbotapi.UpdatesChannel) {
	for update := range updateChan {
		message := update.Message
		text := message.Text
		replyToChatID := message.Chat.ID

		task := &Task{querySearch: text, replyToChatID: replyToChatID}

		if strings.HasPrefix(text, "/") {
			go processCommand(task)
			continue
		}

		// TODO: check for overload, organize pipeline
		go processTask(task)
	}
}

func processCommand(task *Task) {
	switch task.querySearch {
	case "/start":
		replyText := `Введите запрос`
		replyMsg := tgbotapi.NewMessage(task.replyToChatID, replyText)
		botapi.Send(replyMsg)
	}
}

func startTelegram() (updateChan tgbotapi.UpdatesChannel, err error) {
	token := os.Getenv("TELEGRAMBOT_TOKEN")
	botapi, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Printf("Cannot init telegram bot api: %v", err)
		return nil, err
	}

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	// work via webhook
	webhookHost := os.Getenv("TELEGRAMBOT_WEBHOOK_HOST")
	if webhookHost != "" {
		if !strings.Contains(webhookHost, ".") {
			webhookHost += ".herokuapp.com"
		}
		err = setWebhook(fmt.Sprintf("https://%v/%s", webhookHost, token))
		if err != nil {
			log.Fatalf("Error setting webhook: %v", err)
		}
		var info tgbotapi.WebhookInfo
		info, err = botapi.GetWebhookInfo()
		if err != nil {
			log.Fatal(err)
		}
		if info.LastErrorDate != 0 {
			log.Printf("[Telegram callback failed]%s", info.LastErrorMessage)
		}
		updateChan = botapi.ListenForWebhook("/" + token)
		go http.ListenAndServe(":"+os.Getenv("PORT"), nil)
		return
	}

	// work via long polling
	updateChan, err = botapi.GetUpdatesChan(updateConfig)
	if err != nil {
		return nil, errors.Wrap(err, "cannot init tg updates channel")
	}
	return updateChan, nil
}

func processTask(task *Task) error {
	start := time.Now()
	page, err := google(task.querySearch)
	if err != nil {
		return err
	}

	defer page.Close()

	results, err := extractResults(page)
	if err != nil {
		return err
	}

	sendResults(task.replyToChatID, results)

	log.Printf("Time elapsed: %v. Started: %v. Results len: %v", time.Since(start), len(results))
	return nil
}

func extractResults(body io.Reader) (results []Result, err error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse document with goquery")
	}
	doc.Find("h3.r").Each(func(i int, selection *goquery.Selection) {
		a := selection.Find("a").First()
		origURL := a.AttrOr("href", "")
		targetURL := getTargetURL(origURL)
		result := Result{
			name:        a.Text(),
			url:         targetURL,
			pingbackURL: origURL,
		}
		results = append(results, result)
	})
	return results, nil
}

func getTargetURL(origURL string) string {
	u, err := url.Parse(origURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("q")
}

func google(searchQuery string) (io.ReadCloser, error) {
	u := parsedURL
	q := u.Query()
	q.Set("q", searchQuery)
	u.RawQuery = q.Encode()
	client := &http.Client{}
	resp, err := client.Get(u.String())
	if err != nil {
		log.Printf("cannot get page: %v", err)
		return nil, err
	}

	return resp.Body, nil
}

func sendResults(chatID int64, results []Result) {
	for _, r := range results {
		text := fmt.Sprintf(`<a href="%v">%v</a>`+"\n\n", r.url, r.name)
		message := tgbotapi.NewMessage(chatID, text)
		message.ParseMode = "HTML"
		m, err := botapi.Send(message)
		if err != nil {
			log.Printf("Sending error %v. Message was: %v", m, err)
		}
	}
}

func setWebhook(u string) error {
	webhook := tgbotapi.NewWebhook(u)
	_, err := botapi.SetWebhook(webhook)
	if err != nil {
		return errors.Wrap(err, "cannot set webhook")
	}
	return nil
}
