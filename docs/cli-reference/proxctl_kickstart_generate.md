## proxctl kickstart generate

Render kickstart to a file

```
proxctl kickstart generate [ENV_FILE] [flags]
```

### Options

```
  -h, --help          help for generate
      --node string   single node name (default: render all)
  -o, --out string    output directory (default ".")
```

### Options inherited from parent commands

```
      --context string   proxctl context to use (overrides current-context)
      --env string       env manifest name or path (overrides current env)
      --json             emit JSON on stdout (stderr still carries logs)
  -y, --yes              assume yes for confirm prompts (DANGEROUS)
```

### SEE ALSO

* [proxctl kickstart](proxctl_kickstart.md)	 - Render kickstart configs, build + upload install ISOs

