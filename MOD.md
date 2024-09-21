# CommandsSpy
A mod to log commands to the server console. Both existing and non-existing commands are logged. Commands that are permitted or not are added to the server logs as well.

## Installation
Simply place the mod into the mods directory of your server, and it will start monitoring executed commands.
On startup, the config file will be created automatically.

## Examples
Player "Ultra_MC" executing "/gamemode creative" command is logged without arguments in server logs when using config `"logArguments": false` (default):
```
[18:57:06] [Server thread/INFO]: [CommandsSpy] [Player: Ultra_MC] gamemode
```

Player "Ultra_MC" executing "/gamemode creative" command is logged with arguments in server logs when using config `"logArguments": true`:
```
[18:57:06] [Server thread/INFO]: [CommandsSpy] [Player: Ultra_MC] gamemode creative
```

Player "Ultra_MC" executing "/op" command is logged in server logs:
```
[18:56:38] [Server thread/INFO]: [CommandsSpy] [Player: Ultra_MC] op
```

Server console executing "/list" is logged in server logs:
```
[18:53:02] [Server thread/INFO]: [CommandsSpy] [Server] list
```

RCON executing "/save-all" is logged in server logs:
```
[09:25:01] [Server thread/INFO]: [CommandsSpy] [Rcon] save-all
```

## Configuration
### Config file `config/commands-spy.json`
#### Initial config
```
{
"blacklist": [],
"logArguments": false
}
```

#### blacklist
To prevent logging certain commands, add them to the blacklist.
Example - to maintain privacy of players' conversations, you can avoid logging commands `tell` and `t`.
```
"blacklist": ["tell", "t"]
```

#### log arguments
Command arguments are not logged by default: `"logArguments": false`.
Example: `/a b c` - `[CommandsSpy] [Player: Ultra_MC] a`

To log arguments of all commands, use `"logArguments": true`.
Example: `/a b c` - `[CommandsSpy] [Player: Ultra_MC] a b c`
