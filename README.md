# Notes about this fork

Motivated because Signal data formats change over time, and the original fork seems to have been abandoned.

This fork has been tested with a Windows 10 build against Signal backup files that were created on these dates:
* 2022-02-05
* 2022-12-12
* 2024-06-08

Binaries for this tool have not yet been updated in github. You'll need to rebuild from source.

## Building
You must have [Go](https://golang.org) and [dep](https://github.com/golang/dep) installed.

The following steps will create binaries:

     git clone https://github.com/sean-gugler/signal-back
     go get
     build_all.sh 

Below is the documentation from the upstream author xeals. Parts have been updated for this fork, but others are significantly outdated.

---


# signal-back

[![Build status](https://travis-ci.org/xeals/signal-back.svg?branch=master)](https://travis-ci.org/xeals/signal-back)

Since version 4.17.5, the Signal Android app encrypts its backup files. While these are undoubtedly a security benefit over unencrypted backups, they do present an issue in being read into other systems or simply by their owner.

`signal-back` can extract and format the contents of such backup files. You need to specify the file's password; `signal-back` makes no attempt to figure out the password itself.

# Usage

Either [build from source](#building-from-source) or download a [pre-built binary](https://github.com/sean-gugler/signal-back/releases) and put the executable somewhere you can find it.

```
Usage: signal-back COMMAND [OPTION...] BACKUPFILE

  --help, -h     show help
  --version, -v  print the version

Commands:
  analyse  Information about the backup file
  extract  Decrypt contents into individual files
  format   Export messages from a signal database
  help     Shows a list of commands or help for one command
```

Supported export formats are:
- XML: Viewable with a web browser
- CSV: Comma-Separated Value text file
- JSON: JavaScript Object Notation file

Use the `--help` option with any of the commands to see more information about that command.
```sh
signal-back analyze --help
```

# Password

The password you need to decrypt the content of the Signal backup file was shown to you by Signal when you enabled local backups [similar to this screenshot](https://user-images.githubusercontent.com/8427572/36796616-d9560ee6-1c9d-11e8-8440-99e7f5f2ee03.JPG). It consists of six groups of five digits.

The most secure way to provide the password to `signal-back` is to enter it in the interactive dialog such as `12345 12345 12345 12345 12345 12345`. Your entry will not be echoed to the console nor stored anywhere.

For convenience, if you prefer, you can also provide the password on the command line using `-p 12345 12345 12345 12345 12345 12345` or you can write it in a text file and pass it to signal-back using `-P password.txt`.

# Example usage

Download whichever binary suits your system from the [releases page](https://github.com/sean-gugler/signal-back/releases); Windows, Mac OS (`darwin`), or Linux, and 32-bit (`386`) or 64-bit (`amd64`). Checksums are provided to verify file integrity.

Find where you downloaded the file and open an interactive shell (Command Prompt, Terminal.app, gnome-terminal, etc.). Make sure your `signal-XXX.backup` file is in the same folder, or else specify the full path your backup file when running the program.

If you're on MacOS or Linux (where e.g., `OS` is `darwin` and `ARCH` is `amd64`) you may need to use `chmod` to make it possible to run the downloaded file:

```sh
chmod +x signal-back_OS_ARCH
```

In the examples below, the app will be referred to as simply `signal-back`. You should substitute the actual name of the binary you downloaded or built.

## Extracting

All messages are stored in a sqlite3 database file. Attachment files such as images, videos, and PDFs will be placed in a subfolder named `Attachments`.

```sh
signal-back extract -o folder signal-XXX.backup
```

Enter your 30-digit password at the prompt (with or without spaces, doesn't matter). Note that your password will not be echoed back to you for security purposes.

Everything will be extracted to the folder you specified. If you omitted the `-o` option, they'll be in the folder where you ran the command. Note that some attachments may have a `.unknown` extension; this is because `signal-back` might not be able to determine what type of files these are. Please report an issue on github if you encounter one of these.

## Formatting

Once you have extracted the database, you can convert its contents into other formats.

Change directory to where you extracted the database and attachments, then run the formatter.

```sh
cd folder
signal-back format -o messages.xml signal.db
```

### Viewing with a web browser

Find the XSL files in the `xsl` folder of this source repository. Copy them into the same folder as your new XML file.

NOTE: modern browsers will refuse to view the file locally with a `file://` protocol. They need to be served by an actual web server. If you have Python there is a very simple way to start a local server - BE CAREFUL about running this on a computer that is reachable from the Internet; you may be exposing yourself to security risks, for which I cannot be held responsible.

```sh
cd folder
python -m http.server 8000
```

You can then use a web browser on the same computer by visiting the special location 127.0.0.1:8000 and see all the messages in your Signal backup formatted for easy reading.

### Importing to SMS Backup & Restore

If your Signal backup file was created in 2022 or earlier, the XML file can also be imported by [Synctech SMS Backup & Restore](https://www.synctech.com.au/sms-backup-restore/). Newer backups have a revised format that is incompatible (see signalapp commit [e9d98b7](https://github.com/signalapp/Signal-Android/commit/e9d98b7d39ebf147de1138690cca270604cd793e)), and this tool does not attempt to convert it.

Make sure you use the `--embed-attachments` option if you want to include message attachments. This will take longer and result in a larger XML file.

```sh
signal-back format --embed-attachments -o backup.xml signal.db
```

Copy the `backup.xml` file to your phone and restore it using SMS Backup & Restore.

# Building from source

Building requires [Go](https://golang.org) and [dep](https://github.com/golang/dep). If you don't have one (or both) of these tools, instructions should be easy to find. After you've initialised everything:

```
$ git clone https://github.com/sean-gugler/signal-back $GOPATH/src/github.com/sean-gugler/signal-back
$ cd $GOPATH/src/github.com/sean-gugler/signal-back
$ dep ensure
$ go build .
```

You can also just use `go get github.com/sean-gugler/signal-back`, but I provide no guarantees on dependency compatibility.

# Todo list

- [ ] Code cleanup
  - [ ] make code legible for other people
- [x] Actual command line-ness
- [x] Formatting ideas and options
- [ ] User-friendliness in errors and stuff
- [ ] Faster XML display in web browser

# License

Licensed under the Apache License, Version 2.0 ([LICENSE](LICENSE)
or http://www.apache.org/licenses/LICENSE-2.0).

## Contribution

Unless you explicitly state otherwise, any contribution intentionally submitted
for inclusion in the work by you, as defined in the Apache-2.0 license, shall be
licensed as above, without any additional terms or conditions.
