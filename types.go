package main

import "github.com/go-telegram-bot-api/telegram-bot-api"

type Telegramer interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	GetUpdatesChan(config tgbotapi.UpdateConfig) (tgbotapi.UpdatesChannel, error)
	SetWebhook(config tgbotapi.WebhookConfig) (tgbotapi.APIResponse, error)
	ListenForWebhook(string) tgbotapi.UpdatesChannel
	GetWebhookInfo() (tgbotapi.WebhookInfo, error)
}

type Task struct {
	querySearch   string
	result        string //TODO
	error         string
	replyToChatID int64
}

type chanTask chan *Task

type Result struct {
	name        string
	url         string
	pingbackURL string
}
