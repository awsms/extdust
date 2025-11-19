# extdust

**extdust** is a command-line tool that scans files in a directory, groups them by extension, and shows how much space each extension uses. It can also list the biggest files or folders per extension.

`extdust` uses [`fdfind`](https://github.com/sharkdp/fd) for fast file discovery.

---

## Installation

```bash
git clone https://github.com/awsms/extdust.git
cd extdust
go build -o extdust
```

Requires:

* Go 1.18+
* `fd` or `fdfind` in your `$PATH`

---

## Basic Usage

### Scan the current directory (all extensions)

```bash
extdust
```

### Filter by extension(s)

```bash
extdust -e go,md,txt
```

### Show biggest files per extension

```bash
extdust -f
```

### Show biggest folders per extension

```bash
extdust -d
```

### Limit results

```bash
extdust -f -l 20
```

### Sorting

```bash
extdust -s     # size, smallest first
extdust -n     # sort by extension name
```

### Show total size across all extensions

```bash
extdust -t
```

### Combine options

```bash
extdust -e go,md -f -d -l 10
```

---

## License

This project is licensed under the [GPL-3.0 License](LICENSE).