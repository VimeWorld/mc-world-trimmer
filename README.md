# mc-world-trimmer
Optimizer for Minecraft 1.8.8 worlds - trim chunks, remove player data, etc.

```
Usage:
  mc-world-trimmer [options] path
Examples:
  mc-world-trimmer -r -dry .minecraft/saves
  mc-world-trimmer -r -o .
  mc-world-trimmer -o world.zip
Options:
  -dry
        Dry run (no changes on disk)
  -hm
        Recalculate height maps
  -lm
        Compute low maps
  -o    Overwrite original world
  -r    Recursive search for worlds
  -s string
        Suffix for optimized worlds (default "_opt")
  -v    Verbose logging
```
