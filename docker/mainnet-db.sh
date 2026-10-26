# Non-secret defaults for docker/docker-compose.yml. Export before starting the bot.
OPBNB_BRIDGE_BOT_DB_HOST=127.0.0.1
OPBNB_BRIDGE_BOT_DB_PORT=3306
OPBNB_BRIDGE_BOT_DB_USER=db_username
OPBNB_BRIDGE_BOT_DB_PASSWORD=db_password
OPBNB_MAINNET=https://opbnb-mainnet-rpc.bnbchain.org
# Set OPBNB_BRIDGE_BOT_PRIVKEY separately in your environment or secret manager.
# Do not put a signing key in this file.
