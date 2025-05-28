# DDCUtil Daemon

A daemon that controls external monitor brightness and power using DDC/CI protocol. It debounces rapid commands from waybar to prevent spam.

## Commands

Send commands to `/tmp/brightness.sock`:

- `get` - Returns current brightness as JSON: `{"percentage": 75}`
- `inc` - Increment brightness (debounced)
- `dec` - Decrement brightness (debounced) 
- `sleep` - Put monitor into sleep mode
- `wakeup` - Wake up monitor

## Usage
```
echo "get" | nc -U /tmp/brightness.sock
echo "inc" | nc -U /tmp/brightness.sock
echo "dec" | nc -U /tmp/brightness.sock
echo "sleep" | nc -U /tmp/brightness.sock
echo "wakeup" | nc -U /tmp/brightness.sock
```

## Default Configuration

- Socket path: `/tmp/brightness.sock`
- Debounce time: 300ms
- Brightness step: 10 units per increment/decrement
- Monitor ID: 1 (first external monitor)
- Waybar signal: SIGRTMIN+5
