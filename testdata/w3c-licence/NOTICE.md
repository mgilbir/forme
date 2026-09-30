# The W3C Software and Document License

`software-license-2023.html` is https://www.w3.org/copyright/software-license-2023/
as W3C served it on 2026-09-30, byte for byte, with SHA-256
`fd5a4ac6381278e3c62ba92fa3998738f9aefa6760da4953e214ba5ab7341a4e`.

`THIRD_PARTY_NOTICES` quotes the licence from it, under the entry for CSS Color
Module Level 4, and `cmd/notices_test.go` holds the quotation to it.

## Why it is committed

It used to be fetched by `make notice-sources`, pinned by its digest. The page
has no versioned URL, and it changes whenever W3C's site does while the licence
on it does not: on 2026-09-29 a banner for W3C's community survey, on
2026-09-30 the `?ver=` of its stylesheets and scripts. Each change made the
fetch fail its digest, and every CI job with it, before a test ran.

A new version of the licence would have a new URL, as the 2023 one did. To see
whether this page's text has moved: fetch the URL and run
`TestEveryNoticeIsQuotedFromItsSource` against it in place of this file.
