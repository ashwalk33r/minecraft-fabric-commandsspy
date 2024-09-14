# CommandsSpy
Mod to log commands to server console. Existing and non-existing commands are logged. Commands that are permitted or not are added to the server logs too.

## Configuration/Installation
Just put the mod into the mods directory of your server and it will start spying on commands executed. It requires no extra configuration.

## Examples
Player "Ultra_MC" executing "/gamemode creative" command is added into server logs:
```
[18:57:06] [Server thread/INFO]: [CommandsSpy] [Player: Ultra_MC] gamemode creative
```

Player "Ultra_MC" executing "/op" command is added into server logs:
```
[18:56:38] [Server thread/INFO]: [CommandsSpy] [Player: Ultra_MC] op
```

Server console executing "/list" is added into server logs:
```
[18:53:02] [Server thread/INFO]: [CommandsSpy] [Server] list
```

RCON executing "/save-all" is added into server logs:
```
[09:25:01] [Server thread/INFO]: [CommandsSpy] [Rcon] save-all
```
