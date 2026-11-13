## [1.7.0](https://github.com/itsvlxd/StackLog/compare/v1.6.0...v1.7.0) (2026-11-13)

### Features

* **progress:** a bar that hangs off the end of a log line ([8df149e](https://github.com/itsvlxd/StackLog/commit/8df149eae2acf862b646abbe08d32180553d3268)), closes [#31](https://github.com/itsvlxd/StackLog/issues/31)

## [1.6.0](https://github.com/itsvlxd/StackLog/compare/v1.5.0...v1.6.0) (2026-11-12)

### Features

* **config:** a colour policy for writers that are not terminals ([0ae3810](https://github.com/itsvlxd/StackLog/commit/0ae381039df35f27b56ae0925bd754745742e4f1))

## [1.5.0](https://github.com/itsvlxd/StackLog/compare/v1.4.1...v1.5.0) (2026-11-11)

### Features

* **log:** Plain writes a payload with nothing added to it ([db3ee7d](https://github.com/itsvlxd/StackLog/commit/db3ee7dcc1c40a5a2a4a6c6a364d923e4470aa92))

## [1.4.1](https://github.com/itsvlxd/StackLog/compare/v1.4.0...v1.4.1) (2026-11-11)

### Performance Improvements

* **colors:** build each escape once and copy it into the buffer in one write ([cd4be9a](https://github.com/itsvlxd/StackLog/commit/cd4be9a2830541c0353efbf907ce3bf849120c7a))
* **json:** copy runs of ordinary text in one move rather than escaping byte by byte ([3c1a897](https://github.com/itsvlxd/StackLog/commit/3c1a897d8d727f57332225e9bc9c1e2746a9118c))

## [1.4.0](https://github.com/itsvlxd/StackLog/compare/v1.3.0...v1.4.0) (2026-11-11)

### Features

* **buffer:** make the reserved tail a property of the buffer ([b3671ba](https://github.com/itsvlxd/StackLog/commit/b3671ba0ed982dfefc8fab3f5e3a547ec99d1d74))
* **log:** one JSON object per call, cut where a cut cannot break it ([c19d3d1](https://github.com/itsvlxd/StackLog/commit/c19d3d1a7a16f6f9aa97ecfcc156657beb5c051b))

## [1.3.0](https://github.com/itsvlxd/StackLog/compare/v1.2.1...v1.3.0) (2026-11-10)

### Features

* **log:** Config.MinLevel drops every call below the level it keeps ([556fd62](https://github.com/itsvlxd/StackLog/commit/556fd62563d43677853680641eaee846d27d678e)), closes [#14](https://github.com/itsvlxd/StackLog/issues/14)

## [1.2.1](https://github.com/itsvlxd/StackLog/compare/v1.2.0...v1.2.1) (2026-11-10)

### Bug Fixes

* **log:** flush a buffered writer before Fatal exits ([e388afa](https://github.com/itsvlxd/StackLog/commit/e388afaa60325dcf104e77369b64e779f59a727c)), closes [#11](https://github.com/itsvlxd/StackLog/issues/11)

## [1.2.0](https://github.com/itsvlxd/StackLog/compare/v1.1.3...v1.2.0) (2026-11-10)

### Features

* **buffer:** float, duration and error values in the key/value path ([2a7ab28](https://github.com/itsvlxd/StackLog/commit/2a7ab282c961b4f661075b07d2abb8e4da069951)), closes [#23](https://github.com/itsvlxd/StackLog/issues/23) [#15](https://github.com/itsvlxd/StackLog/issues/15)

## [1.1.3](https://github.com/itsvlxd/StackLog/compare/v1.1.2...v1.1.3) (2026-11-10)

### Performance Improvements

* **style:** stop re-emitting a colour the terminal already holds ([3d0f3fe](https://github.com/itsvlxd/StackLog/commit/3d0f3fe2f9c9112ca86567c0fb5f75c15dfc9a7d)), closes [#10](https://github.com/itsvlxd/StackLog/issues/10)

## [1.1.2](https://github.com/itsvlxd/StackLog/compare/v1.1.1...v1.1.2) (2026-11-10)

### Bug Fixes

* **buffer:** give each derived logger its own buffer ([7f3c4c1](https://github.com/itsvlxd/StackLog/commit/7f3c4c18d4c2215803927d424ba617305e87a492)), closes [#12](https://github.com/itsvlxd/StackLog/issues/12)

## [1.1.1](https://github.com/itsvlxd/StackLog/compare/v1.1.0...v1.1.1) (2026-11-10)

### Bug Fixes

* **module:** retract v1.0.1 ([2565074](https://github.com/itsvlxd/StackLog/commit/25650749bdcd172dd30d1eabd091afa1426819a5))

## [1.1.0](https://github.com/itsvlxd/StackLog/compare/v1.0.0...v1.1.0) (2026-11-10)

### ⚠ BREAKING CHANGES

* **module:** the import path moved to github.com/itsvlxd/stacklog/v2 and moved
back the same day. It is github.com/itsvlxd/stacklog. Anything pinned to
v2.0.0 during those 45 minutes has to move back to the unsuffixed path.
* **presets:** stacklog.PresetPlain is removed and replaced by PresetMuted. It
was named in a release that was withdrawn the same day, so nobody can be using
it by name.
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
