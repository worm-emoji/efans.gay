# efans.gay

a simple web server that displays MOTD text. can be updated via discord slash command.

## running locally

1. clone the repo
2. create a `.env` file with required environment variables
3. run `go run main.go`
4. server will start on localhost:4331

## required environment variables

the following must be set in `.env`:

- `DISCORD_APPLICATION_ID`: your discord application id
- `DISCORD_PUBLIC_KEY`: public key from discord application
- `DISCORD_BOT_TOKEN`: bot token from discord application
- `DISCORD_GUILD_ID`: id of the discord server where commands will be registered

these can be obtained by:

1. creating a discord application at https://discord.com/developers/applications
2. creating a bot for the application
3. enabling message content intent for the bot
4. adding the bot to your server with appropriate permissions

the bot will register a `/gay` slash command that can be used to update the displayed text.
