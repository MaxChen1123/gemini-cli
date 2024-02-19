# gemini-cli

`gemini-cli` is a simple yet powerful command-line interface for Google's Gemini LLMs,
written in Go. It includes tools for chatting with these models and
generating / comparing embeddings, with powerful SQLite storage and analysis capabilities.

## Installing

Install `gemini-cli` on your machine with:

```
$ go install github.com/eliben/gemini-cli@latest
```

You can then invoke `gemini-cli help` to verify it's properly installed and found.

## Usage

All `gemini-cli` invocations require the API key for https://ai.google.dev/ to be provided,
either via the `--key` flag or an environment variable called `GEMINI_API_KEY`. From here
one, all examples assume environment variable was set earlier to a valid key.

`gemini-cli` has a nested tree of subcommands to perform various tasks. You can always
run `gemini-cli help <command> [subcommand]...` to get usage; e.g. `gemini-cli help chat`
or `gemini-cli help embed similar`. The printed help information will describe every
subcommand and its flags.

This guide will discuss some of the more common use cases.

### Models

The list of Gemini models supported by the backend is available on [this page](https://ai.google.dev/models/gemini).
You can run `gemini-cli models` to ask the tool to print a list of model names it's familiar
with. These are the names you can pass in with the `--model` flag (see the default model
name by running `gemini-cli help`), and you can always omit the `models/` prefix.

### `prompt` - single prompts

The `prompt` command allows one to send queries consisting of text or images to the LLM.
This is a single-shot interaction; the LLM will have no memory of previous prompts (see
that `chat` command for with-memory interactions).

```
$ gemini-cli prompt <prompt or '-'>... [flags]
```

The prompt can be provided as a sequence of parts, each one a command-line argument.

The arguments are sent as a sequence to the model in the order provided.
If `--system` is provided, it's prepended to the other arguments. An argument
can be some quoted text, a name of an image file on the local filesystem or
a URL pointing directly to an image file online. A special argument with
the value `-` instructs the tool to read this prompt part from standard input.
It can only appear once in a single invocation.

Some examples:

```
# Simple single prompt
$ gemini-cli prompt "why is the sky blue?"

# Multi-modal prompt with image file. Note that we have to ask for a
# vision-capable model explicitly
$ gemini-cli prompt --model gemini-pro-vision "describe this image:" test/datafiles/puppies.png
```

### `chat` - in-terminal chat with a model

Running `gemini-cli chat` starts an interactive terminal chat with a model. You write
prompts following the `>` character and the model prints it replies. In this mode, the model
has a memory of your previous prompts and its own replies (within the model's context length
limit). Example:

```
$ gemini-cli chat
Chatting with gemini-1.0-pro
Type 'exit' or 'quit' to exit
> name 3 dog breeds
1. Golden Retriever
2. Labrador Retriever
3. German Shepherd
> which of these is the heaviest?
German Shepherd

German Shepherds are typically the heaviest of the three breeds, with males
[...]
> and which are considered best for kids?
**Golden Retrievers** and **Labrador Retrievers** are both considered excellent
[...]
> 
```

### `counttok` - counting tokens

We can ask the Gemini API to count the number of tokens in a given prompt or list of prompts.
`gemini-cli` supports this with the `counttok` command. Examples:

```
$ gemini-cli counttok "why is the sky blue?"

$ cat textfile.txt | gemini-cli counttok -
```

### Embeddings

Some of `gemini-cli`'s most advanced capabilities are in interacting with Gemini's embedding
models. `gemini-cli` uses SQLite to store embeddings for a potentially large number of inputs
and query these embeddings for similarity. This is all done through subcommands of the `embed`
command.

#### `embed content` - embedding a single piece of content

Useful for kicking the tires of embeddings, this subcommand lets us embed a single prompt taken
from the command-line or a file, and print out its embedding in various formats (controlled with
the `--format` flag). Examples:

```
$ gemini-cli embed content "why is the sky blue?"

$ cat textfile.txt | gemini-cli embed content -
```

#### `embed db` - embedding multiple contents, storing results in a DB

`embed db` is a swiss-army knife subcommand for embedding multiple pieces of text and storing the
results in a SQLite DB. It supports different kinds of inputs: a table listing contents,
the file system or the DB itself.

All variations of `embed db` take the path of a DB file to use as output. If the file exists, it's
expected to be a valid SQLite DB; otherwise, a new DB is created in that path. `gemini-cli` will
store the results of embedding calculations in this DB in the `embeddings` table (this name can
be configured with the `--table` flag), with this SQL schema:

```
id TEXT PRIMARY KEY
embedding BLOB
```

The `id` is taken from the input, based on its type. We'll go through the different variants of
input next.

**Filesystem input**: when passed the `--files` or `--files-list` flag, `gemini-cli` takes inputs
as files from the filesystem. Each file is one input: its path is the ID, and its contents are
passed to the embedding model.

With `--files`, the flag value is a comma-separated pair of `<root directory>,<glob pattern>`;
the root directory is walked recursively and every file matching the glob pattern is included
in the input. For example:

```
$ gemini-cli embed db out.db --files somedir,*.txt
```

Embeds every `.txt` file found in `somedir` or any of its sub-directories. The ID for each file will
be its path starting with `dir/`.

With `--files-list`, the flag value is a comma-separated pair of filenames. Each name becomes an
ID and the file's concents are passed to the embedding model. This can be useful for more
sophisticated patterns that are difficult to express using a simple glob; for example, using
[pss](https://github.com/eliben/pss/) and the `paste` command, this embeds any file that looks
like a C++ file (i.e. ending with `.h`, `.hpp`, `.cpp`, `.cxx` and so on) in the current directory:

```
$ gemini-cli embed db out-db --files-list $(pss -f --cpp | paste -sd,)
```




## Acknowledgements

`gemini-cli` is inspired by Simon Willison's [llm tool](https://llm.datasette.io/en/stable/), but
aimed at the Go ecosystem. [Simon's website](https://simonwillison.net/) is a treasure trove of
information about LLMs, embeddings and building tools that use them - check it out!
