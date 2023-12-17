# evtail - tail windows event log

Like Unix `tail -f /var/log/syslog`, but for the Windows event log.

Output is grepable (one message per line).

## Usage

```text
evtail.exe -n <name> -t <time (optional)>

  -n string
        name of the windows event log channel (i.e. System, Application, ... - see powershell "get-winevent -listlog *"). (default "Application")
  -t int
        display recent events from last N minutes (defaults to 24 hours) (default 1440)
```

You can get the list of available event log channels using PowerShell:

```powershell
get-winevent -listlog *
```

## Example

Show `Application` channel messages from last 24 hours and tail for new events (these are the defaults):

```shell
$ evtail
2024-01-01 00:00:00.0000000 +0000 UTC HOSTNAME.megacorp.local Application 1704 0 Information SceCli Security policy in the Group policy objects has been applied successfully.
2024-01-01 00:01:00.0000000 +0000 UTC HOSTNAME.megacorp.local Application 0 0 Information edgeupdate Service stopped.
```

Show `System` channel messages from last 10 minutes:

```shell
evtail -n System -t 10
```

Show messages from some "special" channel (with 15 minutes backlog):

```shell
evtail -n "Microsoft-Windows-BackgroundTaskInfrastructure/Operational" -t 15
```

## Attributions

This is a quick hack on top of [ofcoursedude/gowinlog](https://github.com/ofcoursedude/gowinlog) (vendored here), which seems to originate from [scalingdata/gowinlog](https://github.com/scalingdata/gowinlog).
