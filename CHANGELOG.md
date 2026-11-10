## [1.1.0](https://github.com/itsvlxd/StackLog/compare/v1.0.0...v1.1.0) (2026-11-10)

### ⚠ BREAKING CHANGES

* **module:** the import path is now github.com/itsvlxd/stacklog/v2.
* **presets:** stacklog.PresetPlain is removed and replaced by PresetMuted. It
was released in 2.0.0, so anyone using it by name breaks.
* **colors:** callers who used Background(0) as "no background" now get a black
background. The predefined styles were changed to stop calling Background(0), so
the package default is unchanged.

addPostStyle writes the reset only for a component that actually opened a colour.
Without this a style with nothing set still emits an escape sequence, which is what
makes a plain preset plain.

### Features

* **colors:** stop overloading 0 as "not set" in CompStyle ([0989190](https://github.com/itsvlxd/StackLog/commit/0989190c4744d469b2623fb8c54146c3c5ab2bfd))
* **presets:** add named style presets and a Prefix field ([92cb7a1](https://github.com/itsvlxd/StackLog/commit/92cb7a19e61a17426c00c7412c932f057145d903))
* **presets:** add per-preset level labels through Style.LevelPrefix ([997b4ef](https://github.com/itsvlxd/StackLog/commit/997b4ef479f54377243898346c63d5dda4edb463))
* **presets:** every preset carries a palette, and NoColors owns the flag ([5f79718](https://github.com/itsvlxd/StackLog/commit/5f79718d1c9719d570a59f052dc836184b15412a))

### Bug Fixes

* **buffer:** reserve the tail so a truncated line keeps its terminator ([0cabaf5](https://github.com/itsvlxd/StackLog/commit/0cabaf5a0519bb911e76512c109695cf4f6ddf49))
* **colors:** correct the ANSI16 boundaries ([249e91a](https://github.com/itsvlxd/StackLog/commit/249e91a64b329e3d2e189c8641328e47682f46e1))
* **module:** match the import path to the major version ([db4c2a7](https://github.com/itsvlxd/StackLog/commit/db4c2a753b089b9bd5a0c7647ca87fbc8677c0d4)), closes [#24](https://github.com/itsvlxd/StackLog/issues/24)
* **module:** return to the unsuffixed import path ([b4ab42b](https://github.com/itsvlxd/StackLog/commit/b4ab42bec5c0c743eb71fc72db81d55ab6ee7b3d)), closes [#24](https://github.com/itsvlxd/StackLog/issues/24)
* **stacklog:** add checks for level and params style ([07ea2fe](https://github.com/itsvlxd/StackLog/commit/07ea2fe5863c847d7d1af1e5b98f172093e585d5))
* **style:** clamp colour values to the range Color can hold ([66be39c](https://github.com/itsvlxd/StackLog/commit/66be39c6ea6c9c250e11b2bdee8faa1898aacff7))

## 1.0.0 (2026-11-05)

### Features

* **release:** initial commit ([8aabf09](https://github.com/itsvlxd/StackLog/commit/8aabf09c4943723975004d090793bd7f4b1b5ee6))
