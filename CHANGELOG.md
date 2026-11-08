## [3.0.0](https://github.com/itsvlxd/StackLog/compare/v2.0.0...v3.0.0) (2026-11-08)

### ⚠ BREAKING CHANGES

* **presets:** stacklog.PresetPlain is removed and replaced by PresetMuted. It
was released in 2.0.0, so anyone using it by name breaks.

### Features

* **presets:** every preset carries a palette, and NoColors owns the flag ([ad179a8](https://github.com/itsvlxd/StackLog/commit/ad179a825ee5baa5205b791aa4329f693de098f5))

## [2.0.0](https://github.com/itsvlxd/StackLog/compare/v1.0.1...v2.0.0) (2026-11-08)

### ⚠ BREAKING CHANGES

* **colors:** callers who used Background(0) as "no background" now get a black
background. The predefined styles were changed to stop calling Background(0), so
the package default is unchanged.

addPostStyle writes the reset only for a component that actually opened a colour.
Without this a style with nothing set still emits an escape sequence, which is what
makes a plain preset plain.

### Features

* **colors:** stop overloading 0 as "not set" in CompStyle ([279653f](https://github.com/itsvlxd/StackLog/commit/279653f42bf6a8fac0b23d259d397be6b296f0c8))
* **presets:** add named style presets and a Prefix field ([a278b92](https://github.com/itsvlxd/StackLog/commit/a278b92dd425b859885ae00591f31e8d69090f55))
* **presets:** add per-preset level labels through Style.LevelPrefix ([0583c7a](https://github.com/itsvlxd/StackLog/commit/0583c7a3ee7fcc01578b84c62699869bee805ef6))

## [1.0.1](https://github.com/itsvlxd/StackLog/compare/v1.0.0...v1.0.1) (2026-11-08)

### Bug Fixes

* **stacklog:** add checks for level and params style ([07ea2fe](https://github.com/itsvlxd/StackLog/commit/07ea2fe5863c847d7d1af1e5b98f172093e585d5))

## 1.0.0 (2026-11-05)

### Features

* **release:** initial commit ([8aabf09](https://github.com/itsvlxd/StackLog/commit/8aabf09c4943723975004d090793bd7f4b1b5ee6))
