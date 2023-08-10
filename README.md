# Monobank personal telegram bot
[![GitHub release](https://img.shields.io/github/release/vkopitsa/mono_personal_tgbot.svg)]()
[![Test](https://github.com/vkopitsa/mono_personal_tgbot/actions/workflows/test.yml/badge.svg)](https://github.com/vkopitsa/mono_personal_tgbot/actions/workflows/test.yml)
![Download](https://img.shields.io/github/downloads/vkopitsa/mono_personal_tgbot/total.svg)
[![license](https://img.shields.io/github/license/vkopitsa/mono_personal_tgbot.svg)]()

A simple telegram bot, written in Go with the [telegram-bot-api](https://github.com/go-telegram-bot-api/telegram-bot-api 'telegram-bot-api') library.

![mono_personal_tgbot](Resources/screencast.gif)

![mono_personal_tgbot](Resources/screenshot.png)

![mono_personal_tgbot](Resources/screenshot1.png)

## Usage

Run `mono_personal_tgbot` execution file in your terminal with following env variables

 Environment variable    | Description
------------------------ | -----------------------------------------------------------
`TELEGRAM_TOKEN`         | [How to get telegram bot token](https://core.telegram.org/bots#3-how-do-i-create-a-bot)
`TELEGRAM_ADMINS`        | ids of the trusted user, example: `1234567,1234567`
`TELEGRAM_CHATS`         | ids of the trusted chats, example: `-1234567,-1234567`
`SCHEDULE_TIME`          | set time for daily report, example: `0 21 * * *`
`MONO_TOKENS`            | [How to get monobank token](https://api.monobank.ua/)

### Telegram commands

 Command                 | Description
------------------------ | -----------------------------------------------------------
`/balance`               | Get a balance of the clients.
`/report`                | Get a report for the period of the clients.
`/get_webhook[_n]`       | Get a status about setup webhook of the default client or first one or by number. example: `/get_webhook`, `/get_webhook_1`
`/set_webhook[_n]`       | Set webhook url to monobank api of the default client or first one or by number. example: `/set_webhook`, `/set_webhook_1`


## Usage with docker-compose

Rename `.env.dev` file to `.env` and edit.

    # docker-compose up -d

## Download
[Windows, Linux, MacOS](https://github.com/vkopitsa/mono_personal_tgbot/releases)

## Compatibility
The bot is only available for Windows, Linux, MacOS.

### Licence
The bot is available under the MIT license. See the [LICENSE file](https://github.com/vkopitsa/mono_personal_tgbot/blob/master/LICENSE) for more info.
