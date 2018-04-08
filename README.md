# signal-back

In version 4.17.5, the Signal Android app introduced encrypted backups. While these are undoubtedly a security benefit over unencrypted backups, they do present an issue in being read into other systems or simply by their owner.

`signal-back` is intended to use the same decryption process as the Signal app uses when importing its backups, to make them readable without being used by the app.

# Usage

```
Usage: signal-back [OPTION...] BACKUPFILE

  --password PASS, -p PASS    use PASS as password for backup file
  --pwdfile FILE, -P FILE     read password from FILE
  --format FORMAT, -f FORMAT  output the backup as FORMAT
  --attachments, -a           extract attachments from the backup
  --help, -h                  show help
  --version, -v               print the version
```

The current interface is by no means complete and I intend to expand on it with stuff like:

- output directory (for extraction)
- output file (for formatting)

**Currently no formats are available, because I don't know what people might want to see. Please contribute on [#2](https://github.com/xeals/signal-back/issues/2) if you have something you'd like to see!**

# Current progress

The program so far is a relatively literal translation of the Java code into Go.

The code is very messy and not very good Go in some places.

Everything is decryptable (that I'm aware of).

## Todo list

- [ ] Code cleanup
- [x] Actual command line-ness
- [ ] Formatting ideas and options

# License

Licensed under the Apache License, Version 2.0 ([LICENSE](LICENSE)
or http://www.apache.org/licenses/LICENSE-2.0).

## Contribution

Unless you explicitly state otherwise, any contribution intentionally submitted
for inclusion in the work by you, as defined in the Apache-2.0 license, shall be
licensed as above, without any additional terms or conditions.
