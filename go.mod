module github.com/itsvlxd/stacklog

go 1.27

retract v1.0.1 // the ANSI16 branch emits wrong codes for indices 8 through 15 and an incomplete escape sequence. Corrected in v1.1.0.
