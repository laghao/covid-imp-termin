# covid-imp-termin

Telegram bot to get information on vaccine availabilities. Get the latest appointments directly on telegram.

The bot listen to updates from doctolib doctors based on various vaccines types which can be picked.

## Usage

Create a [telegram bot](https://core.telegram.org/bots#creating-a-new-bot) to get the telegram token.

Add the generated telegram bot token and your city's Doctolib agendas to `.config.yml`(The current config file contains Hamburg city doctors).

## copilot

```
copilot init --app covid-imp-termin --name covimp --type 'Load Balanced Web Service' --dockerfile './Dockerfile' --port 80 --deploy
```
