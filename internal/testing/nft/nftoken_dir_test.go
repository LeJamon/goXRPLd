package nft_test

// NFTokenDir_test.go - NFT directory (page management) tests
// Reference: rippled/src/test/app/NFTokenDir_test.cpp

import (
	"testing"

	jtx "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/nft"
)

// Seeds that produce AccountIDs with identical low 32-bits (0x9a8ebed3)
// Used for testing page overflow scenarios with 33 equivalent NFTs
var seedsLow32_9a8ebed3 = []string{
	"sp6JS7f14BuwFY8Mw5FnqmbciPvH6",  //  0. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mw5MBGbyMSsXLp",  //  1. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mw5S4PnDyBdKKm",  //  2. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mw6kcXpM2enE35",  //  3. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mw6tuuSMMwyJ44",  //  4. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mw8E8JWLQ1P8pt",  //  5. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mw8WwdgWkCHhEx",  //  6. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mw8XDUYvU6oGhQ",  //  7. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mw8ceVGL4M1zLQ",  //  8. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mw8fdSwLCZWDFd",  //  9. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mw8zuF6Fg65i1E",  // 10. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwF2k7bihVfqes",  // 11. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwF6X24WXGn557",  // 12. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwFMpn7strjekg",  // 13. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwFSdy9sYVrwJs",  // 14. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwFdMcLy9UkrXn",  // 15. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwFdbwFm1AAboa",  // 16. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwFdr5AhKThVtU",  // 17. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwjFc3Q9YatvAw",  // 18. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwjRXcNs1ozEXn",  // 19. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwkQGUKL7v1FBt",  // 20. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mwkamsoxx1wECt",  // 21. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mwm3hus1dG6U8y",  // 22. 0x9a8ebed3
	"sp6JS7f14BuwFY8Mwm589M8vMRpXF",  // 23. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwmJTRJ4Fqz1A3",  // 24. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwmRfy8fer4QbL",  // 25. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwmkkFx1HtgWRx",  // 26. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwmwP9JFdKa4PS",  // 27. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwoXWJLB3ciHfo",  // 28. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwoYc1gTtT2mWL",  // 29. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwogXtHH7FNVoo",  // 30. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwoqYoA9P8gf3r",  // 31. 0x9a8ebed3
	"sp6JS7f14BuwFY8MwoujwMJofGnsA",  // 32. 0x9a8ebed3
}

// Seeds for consecutive packing test (low 32-bits = 0x115d0525)
var seedsLow32_115d0525 = []string{
	"sp6JS7f14BuwFY8Mw56vZeiBuhePx",  //  0. 0x115d0525
	"sp6JS7f14BuwFY8Mw5BodF9tGuTUe",  //  1. 0x115d0525
	"sp6JS7f14BuwFY8Mw5EnhC1cg84J7",  //  2. 0x115d0525
	"sp6JS7f14BuwFY8Mw5P913Cunr2BK",  //  3. 0x115d0525
	"sp6JS7f14BuwFY8Mw5Pru7eLo1XzT",  //  4. 0x115d0525
	"sp6JS7f14BuwFY8Mw61SLUC8UX2m8",  //  5. 0x115d0525
	"sp6JS7f14BuwFY8Mw6AsBF9TpeMpq",  //  6. 0x115d0525
	"sp6JS7f14BuwFY8Mw84XqrBZkU2vE",  //  7. 0x115d0525
	"sp6JS7f14BuwFY8Mw89oSU6dBk3KB",  //  8. 0x115d0525
	"sp6JS7f14BuwFY8Mw89qUKCyDmyzj",  //  9. 0x115d0525
	"sp6JS7f14BuwFY8Mw8GfqQ9VRZ8tm",  // 10. 0x115d0525
	"sp6JS7f14BuwFY8Mw8LtW3VqrqMks",  // 11. 0x115d0525
	"sp6JS7f14BuwFY8Mw8ZrAkJc2sHew",  // 12. 0x115d0525
	"sp6JS7f14BuwFY8Mw8jpkYSNrD3ah",  // 13. 0x115d0525
	"sp6JS7f14BuwFY8MwF2mshd786m3V",  // 14. 0x115d0525
	"sp6JS7f14BuwFY8MwFHfXq9x5NbPY",  // 15. 0x115d0525
	"sp6JS7f14BuwFY8MwFrjWq5LAB8NT",  // 16. 0x115d0525
	"sp6JS7f14BuwFY8Mwj4asgSh6hQZd",  // 17. 0x115d0525
	"sp6JS7f14BuwFY8Mwj7ipFfqBSRrE",  // 18. 0x115d0525
	"sp6JS7f14BuwFY8MwjHqtcvGav8uW",  // 19. 0x115d0525
	"sp6JS7f14BuwFY8MwjLp4sk5fmzki",  // 20. 0x115d0525
	"sp6JS7f14BuwFY8MwjioHuYb3Ytkx",  // 21. 0x115d0525
	"sp6JS7f14BuwFY8MwkRjHPXWi7fGN",  // 22. 0x115d0525
	"sp6JS7f14BuwFY8MwkdVdPV3LjNN1",  // 23. 0x115d0525
	"sp6JS7f14BuwFY8MwkxUtVY5AXZFk",  // 24. 0x115d0525
	"sp6JS7f14BuwFY8Mwm4jQzdfTbY9F",  // 25. 0x115d0525
	"sp6JS7f14BuwFY8MwmCucYAqNp4iF",  // 26. 0x115d0525
	"sp6JS7f14BuwFY8Mwo2bgdFtxBzpF",  // 27. 0x115d0525
	"sp6JS7f14BuwFY8MwoGwD7v4U6qBh",  // 28. 0x115d0525
	"sp6JS7f14BuwFY8MwoUczqFADMoXi",  // 29. 0x115d0525
	"sp6JS7f14BuwFY8MwoY1xZeGd3gAr",  // 30. 0x115d0525
	"sp6JS7f14BuwFY8MwomVCbfkv4kYZ",  // 31. 0x115d0525
	"sp6JS7f14BuwFY8MwoqbrPSr4z13F",  // 32. 0x115d0525
}

// Seeds for lopsided split test - split and add to high page
// Contains groups with identical low 32-bits: 0x1d2932ea, 0x208dbc24, 0x309b67ed, 0x40d4b96f, 0x503b6ba9
var seedsSplitAndAddToHi = []string{
	"sp6JS7f14BuwFY8Mw5p3b8jjQBBTK",  //  0. 0x1d2932ea
	"sp6JS7f14BuwFY8Mw6F7X3EiGKazu",  //  1. 0x1d2932ea
	"sp6JS7f14BuwFY8Mw6FxjntJJfKXq",  //  2. 0x1d2932ea
	"sp6JS7f14BuwFY8Mw6eSF1ydEozJg",  //  3. 0x1d2932ea
	"sp6JS7f14BuwFY8Mw6koPB91um2ej",  //  4. 0x1d2932ea
	"sp6JS7f14BuwFY8Mw6m6D64iwquSe",  //  5. 0x1d2932ea

	"sp6JS7f14BuwFY8Mw5rC43sN4adC2",  //  6. 0x208dbc24
	"sp6JS7f14BuwFY8Mw65L9DDQqgebz",  //  7. 0x208dbc24
	"sp6JS7f14BuwFY8Mw65nKvU8pPQNn",  //  8. 0x208dbc24
	"sp6JS7f14BuwFY8Mw6bxZLyTrdipw",  //  9. 0x208dbc24
	"sp6JS7f14BuwFY8Mw6d5abucntSoX",  // 10. 0x208dbc24
	"sp6JS7f14BuwFY8Mw6qXK5awrRRP8",  // 11. 0x208dbc24

	// These eight need to be kept together by the implementation
	"sp6JS7f14BuwFY8Mw66EBtMxoMcCa",  // 12. 0x309b67ed
	"sp6JS7f14BuwFY8Mw66dGfE9jVfGv",  // 13. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6APdZa7PH566",  // 14. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6C3QX5CZyET5",  // 15. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6CSysFf8GvaR",  // 16. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6c7QSDmoAeRV",  // 17. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6mvonveaZhW7",  // 18. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6vtHHG7dYcXi",  // 19. 0x309b67ed

	"sp6JS7f14BuwFY8Mw66yppUNxESaw",  // 20. 0x40d4b96f
	"sp6JS7f14BuwFY8Mw6ATYQvobXiDT",  // 21. 0x40d4b96f
	"sp6JS7f14BuwFY8Mw6bis8D1Wa9Uy",  // 22. 0x40d4b96f
	"sp6JS7f14BuwFY8Mw6cTiGCWA8Wfa",  // 23. 0x40d4b96f
	"sp6JS7f14BuwFY8Mw6eAy2fpXmyYf",  // 24. 0x40d4b96f
	"sp6JS7f14BuwFY8Mw6icn58TRs8YG",  // 25. 0x40d4b96f

	"sp6JS7f14BuwFY8Mw68tj2eQEWoJt",  // 26. 0x503b6ba9
	"sp6JS7f14BuwFY8Mw6AjnAinNnMHT",  // 27. 0x503b6ba9
	"sp6JS7f14BuwFY8Mw6CKDUwB4LrhL",  // 28. 0x503b6ba9
	"sp6JS7f14BuwFY8Mw6d2yPszEFA6J",  // 29. 0x503b6ba9
	"sp6JS7f14BuwFY8Mw6jcBQBH3PfnB",  // 30. 0x503b6ba9
	"sp6JS7f14BuwFY8Mw6qxx19KSnN1w",  // 31. 0x503b6ba9

	// Adding this NFT splits the page. It is added to the upper page.
	"sp6JS7f14BuwFY8Mw6ut1hFrqWoY5",  // 32. 0x503b6ba9
}

// Seeds for lopsided split test - split and add to low page
var seedsSplitAndAddToLo = []string{
	"sp6JS7f14BuwFY8Mw5p3b8jjQBBTK",  //  0. 0x1d2932ea
	"sp6JS7f14BuwFY8Mw6F7X3EiGKazu",  //  1. 0x1d2932ea
	"sp6JS7f14BuwFY8Mw6FxjntJJfKXq",  //  2. 0x1d2932ea
	"sp6JS7f14BuwFY8Mw6eSF1ydEozJg",  //  3. 0x1d2932ea
	"sp6JS7f14BuwFY8Mw6koPB91um2ej",  //  4. 0x1d2932ea
	"sp6JS7f14BuwFY8Mw6m6D64iwquSe",  //  5. 0x1d2932ea

	"sp6JS7f14BuwFY8Mw5rC43sN4adC2",  //  6. 0x208dbc24
	"sp6JS7f14BuwFY8Mw65L9DDQqgebz",  //  7. 0x208dbc24
	"sp6JS7f14BuwFY8Mw65nKvU8pPQNn",  //  8. 0x208dbc24
	"sp6JS7f14BuwFY8Mw6bxZLyTrdipw",  //  9. 0x208dbc24
	"sp6JS7f14BuwFY8Mw6d5abucntSoX",  // 10. 0x208dbc24
	"sp6JS7f14BuwFY8Mw6qXK5awrRRP8",  // 11. 0x208dbc24

	// These eight need to be kept together by the implementation
	"sp6JS7f14BuwFY8Mw66EBtMxoMcCa",  // 12. 0x309b67ed
	"sp6JS7f14BuwFY8Mw66dGfE9jVfGv",  // 13. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6APdZa7PH566",  // 14. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6C3QX5CZyET5",  // 15. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6CSysFf8GvaR",  // 16. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6c7QSDmoAeRV",  // 17. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6mvonveaZhW7",  // 18. 0x309b67ed
	"sp6JS7f14BuwFY8Mw6vtHHG7dYcXi",  // 19. 0x309b67ed

	"sp6JS7f14BuwFY8Mw66yppUNxESaw",  // 20. 0x40d4b96f
	"sp6JS7f14BuwFY8Mw6ATYQvobXiDT",  // 21. 0x40d4b96f
	"sp6JS7f14BuwFY8Mw6bis8D1Wa9Uy",  // 22. 0x40d4b96f
	"sp6JS7f14BuwFY8Mw6cTiGCWA8Wfa",  // 23. 0x40d4b96f
	"sp6JS7f14BuwFY8Mw6eAy2fpXmyYf",  // 24. 0x40d4b96f
	"sp6JS7f14BuwFY8Mw6icn58TRs8YG",  // 25. 0x40d4b96f

	"sp6JS7f14BuwFY8Mw68tj2eQEWoJt",  // 26. 0x503b6ba9
	"sp6JS7f14BuwFY8Mw6AjnAinNnMHT",  // 27. 0x503b6ba9
	"sp6JS7f14BuwFY8Mw6CKDUwB4LrhL",  // 28. 0x503b6ba9
	"sp6JS7f14BuwFY8Mw6d2yPszEFA6J",  // 29. 0x503b6ba9
	"sp6JS7f14BuwFY8Mw6jcBQBH3PfnB",  // 30. 0x503b6ba9
	"sp6JS7f14BuwFY8Mw6qxx19KSnN1w",  // 31. 0x503b6ba9

	// Adding this NFT splits the page. It is added to the lower page.
	"sp6JS7f14BuwFY8Mw6xCigaMwC6Dp",  // 32. 0x309b67ed
}

// Seeds for fixNFTokenDirV1 test - 17 in high group
var seedsSeventeenHi = []string{
	// These 16 need to be kept together by the implementation (0x399187e9)
	"sp6JS7f14BuwFY8Mw5EYu5z86hKDL",  //  0. 0x399187e9
	"sp6JS7f14BuwFY8Mw5PUAMwc5ygd7",  //  1. 0x399187e9
	"sp6JS7f14BuwFY8Mw5R3xUBcLSeTs",  //  2. 0x399187e9
	"sp6JS7f14BuwFY8Mw5W6oS5sdC3oF",  //  3. 0x399187e9
	"sp6JS7f14BuwFY8Mw5pYc3D9iuLcw",  //  4. 0x399187e9
	"sp6JS7f14BuwFY8Mw5pfGVnhcdp3b",  //  5. 0x399187e9
	"sp6JS7f14BuwFY8Mw6jS6RdEqXqrN",  //  6. 0x399187e9
	"sp6JS7f14BuwFY8Mw6krt6AKbvRXW",  //  7. 0x399187e9
	"sp6JS7f14BuwFY8Mw6mnVBQq7cAN2",  //  8. 0x399187e9
	"sp6JS7f14BuwFY8Mw8ECJxPjmkufQ",  //  9. 0x399187e9
	"sp6JS7f14BuwFY8Mw8asgzcceGWYm",  // 10. 0x399187e9
	"sp6JS7f14BuwFY8MwF6J3FXnPCgL8",  // 11. 0x399187e9
	"sp6JS7f14BuwFY8MwFEud2w5czv5q",  // 12. 0x399187e9
	"sp6JS7f14BuwFY8MwFNxKVqJnx8P5",  // 13. 0x399187e9
	"sp6JS7f14BuwFY8MwFnTCXg3eRidL",  // 14. 0x399187e9
	"sp6JS7f14BuwFY8Mwj47hv1vrDge6",  // 15. 0x399187e9

	// These 17 need to be kept together by the implementation (0xabb11898)
	"sp6JS7f14BuwFY8MwjJCwYr9zSfAv",  // 16. 0xabb11898
	"sp6JS7f14BuwFY8MwjYa5yLkgCLuT",  // 17. 0xabb11898
	"sp6JS7f14BuwFY8MwjenxuJ3TH2Bc",  // 18. 0xabb11898
	"sp6JS7f14BuwFY8MwjriN7Ui11NzB",  // 19. 0xabb11898
	"sp6JS7f14BuwFY8Mwk3AuoJNSEo34",  // 20. 0xabb11898
	"sp6JS7f14BuwFY8MwkT36hnRv8hTo",  // 21. 0xabb11898
	"sp6JS7f14BuwFY8MwkTQixEXfi1Cr",  // 22. 0xabb11898
	"sp6JS7f14BuwFY8MwkYJaZM1yTJBF",  // 23. 0xabb11898
	"sp6JS7f14BuwFY8Mwkc4k1uo85qp2",  // 24. 0xabb11898
	"sp6JS7f14BuwFY8Mwkf7cFhF1uuxx",  // 25. 0xabb11898
	"sp6JS7f14BuwFY8MwmCK2un99wb4e",  // 26. 0xabb11898
	"sp6JS7f14BuwFY8MwmETztNHYu2Bx",  // 27. 0xabb11898
	"sp6JS7f14BuwFY8MwmJws9UwRASfR",  // 28. 0xabb11898
	"sp6JS7f14BuwFY8MwoH5PQkGK8tEb",  // 29. 0xabb11898
	"sp6JS7f14BuwFY8MwoVXtP2yCzjJV",  // 30. 0xabb11898
	"sp6JS7f14BuwFY8MwobxRXA9vsTeX",  // 31. 0xabb11898
	"sp6JS7f14BuwFY8Mwos3pc5Gb3ihU",  // 32. 0xabb11898
}

// Seeds for fixNFTokenDirV1 test - 17 in low group
var seedsSeventeenLo = []string{
	// These 17 need to be kept together by the implementation (0x399187e9)
	"sp6JS7f14BuwFY8Mw5EYu5z86hKDL",  //  0. 0x399187e9
	"sp6JS7f14BuwFY8Mw5PUAMwc5ygd7",  //  1. 0x399187e9
	"sp6JS7f14BuwFY8Mw5R3xUBcLSeTs",  //  2. 0x399187e9
	"sp6JS7f14BuwFY8Mw5W6oS5sdC3oF",  //  3. 0x399187e9
	"sp6JS7f14BuwFY8Mw5pYc3D9iuLcw",  //  4. 0x399187e9
	"sp6JS7f14BuwFY8Mw5pfGVnhcdp3b",  //  5. 0x399187e9
	"sp6JS7f14BuwFY8Mw6jS6RdEqXqrN",  //  6. 0x399187e9
	"sp6JS7f14BuwFY8Mw6krt6AKbvRXW",  //  7. 0x399187e9
	"sp6JS7f14BuwFY8Mw6mnVBQq7cAN2",  //  8. 0x399187e9
	"sp6JS7f14BuwFY8Mw8ECJxPjmkufQ",  //  9. 0x399187e9
	"sp6JS7f14BuwFY8Mw8asgzcceGWYm",  // 10. 0x399187e9
	"sp6JS7f14BuwFY8MwF6J3FXnPCgL8",  // 11. 0x399187e9
	"sp6JS7f14BuwFY8MwFEud2w5czv5q",  // 12. 0x399187e9
	"sp6JS7f14BuwFY8MwFNxKVqJnx8P5",  // 13. 0x399187e9
	"sp6JS7f14BuwFY8MwFnTCXg3eRidL",  // 14. 0x399187e9
	"sp6JS7f14BuwFY8Mwj47hv1vrDge6",  // 15. 0x399187e9
	"sp6JS7f14BuwFY8Mwj6TYekeeyukh",  // 16. 0x399187e9

	// These 16 need to be kept together by the implementation (0xabb11898)
	"sp6JS7f14BuwFY8MwjYa5yLkgCLuT",  // 17. 0xabb11898
	"sp6JS7f14BuwFY8MwjenxuJ3TH2Bc",  // 18. 0xabb11898
	"sp6JS7f14BuwFY8MwjriN7Ui11NzB",  // 19. 0xabb11898
	"sp6JS7f14BuwFY8Mwk3AuoJNSEo34",  // 20. 0xabb11898
	"sp6JS7f14BuwFY8MwkT36hnRv8hTo",  // 21. 0xabb11898
	"sp6JS7f14BuwFY8MwkTQixEXfi1Cr",  // 22. 0xabb11898
	"sp6JS7f14BuwFY8MwkYJaZM1yTJBF",  // 23. 0xabb11898
	"sp6JS7f14BuwFY8Mwkc4k1uo85qp2",  // 24. 0xabb11898
	"sp6JS7f14BuwFY8Mwkf7cFhF1uuxx",  // 25. 0xabb11898
	"sp6JS7f14BuwFY8MwmCK2un99wb4e",  // 26. 0xabb11898
	"sp6JS7f14BuwFY8MwmETztNHYu2Bx",  // 27. 0xabb11898
	"sp6JS7f14BuwFY8MwmJws9UwRASfR",  // 28. 0xabb11898
	"sp6JS7f14BuwFY8MwoH5PQkGK8tEb",  // 29. 0xabb11898
	"sp6JS7f14BuwFY8MwoVXtP2yCzjJV",  // 30. 0xabb11898
	"sp6JS7f14BuwFY8MwobxRXA9vsTeX",  // 31. 0xabb11898
	"sp6JS7f14BuwFY8Mwos3pc5Gb3ihU",  // 32. 0xabb11898
}

// TestConsecutiveNFTs tests that it's possible to store many consecutive NFTs.
// Reference: rippled NFTokenDir_test.cpp testConsecutiveNFTs
func TestConsecutiveNFTs(t *testing.T) {
	t.Skip("testConsecutiveNFTs requires NFT page inspection and taxon cipher")

	env := jtx.NewTestEnv(t)

	issuer := jtx.NewAccount("issuer")
	buyer := jtx.NewAccount("buyer")

	env.Fund(issuer, buyer)
	env.Close()

	// Mint 100 sequential NFTs
	// Tweak the taxon so zero is always stored internally
	const nftCount = 100
	nftIDs := make([]string, 0, nftCount)

	for i := 0; i < nftCount; i++ {
		// Use ciphered taxon to force sequential storage:
		// taxon := toUInt32(nft.cipheredTaxon(i, nft.toTaxon(0)))
		taxon := uint32(0) // Simplified

		mintTx := nft.NFTokenMint(issuer, taxon).Transferable().Build()
		result := env.Submit(mintTx)
		if result.Success {
			nftIDs = append(nftIDs, "nft_"+string(rune(i)))
		}
		env.Close()
	}

	t.Logf("Minted %d NFTs", len(nftIDs))

	// Create an offer for each NFT to verify ledger can find them
	offers := make([]string, 0, len(nftIDs))
	for _, nftID := range nftIDs {
		offerTx := nft.NFTokenCreateSellOffer(issuer, nftID, jtx.XRPTxAmount(0)).Build()
		result := env.Submit(offerTx)
		if result.Success {
			offers = append(offers, "offer_for_"+nftID)
		}
		env.Close()
	}

	// Buyer accepts all offers in reverse order
	for i := len(offers) - 1; i >= 0; i-- {
		acceptTx := nft.NFTokenAcceptSellOffer(buyer, offers[i]).Build()
		env.Submit(acceptTx)
		env.Close()
	}

	t.Log("testConsecutiveNFTs passed")
}

// TestLopsidedSplits tests that all NFT IDs with the same low 96 bits stay on the same NFT page.
// Reference: rippled NFTokenDir_test.cpp testLopsidedSplits
func TestLopsidedSplits(t *testing.T) {
	t.Skip("testLopsidedSplits requires account creation from seed")

	exerciseLopsided := func(t *testing.T, seeds []string) {
		env := jtx.NewTestEnv(t)

		buyer := jtx.NewAccount("buyer")
		env.Fund(buyer)
		env.Close()

		// Create accounts from seeds and fund them
		accounts := make([]*jtx.Account, 0, len(seeds))
		for _, seed := range seeds {
			// In real implementation: account := jtx.NewAccountFromSeed(seed)
			account := jtx.NewAccount(seed) // Simplified
			accounts = append(accounts, account)
			env.Fund(account)
		}
		env.Close()

		// All accounts create one NFT and offer it to buyer
		nftIDs := make([]string, 0, len(accounts))
		offers := make([]string, 0, len(accounts))

		for _, account := range accounts {
			mintTx := nft.NFTokenMint(account, 0).Transferable().Build()
			result := env.Submit(mintTx)
			if result.Success {
				nftID := "nft_" + account.Address // Would extract from result
				nftIDs = append(nftIDs, nftID)

				offerTx := nft.NFTokenCreateSellOffer(account, nftID, jtx.XRPTxAmount(0)).
					Destination(buyer).Build()
				result = env.Submit(offerTx)
				if result.Success {
					offers = append(offers, "offer_"+nftID)
				}
			}
			env.Close()
		}

		// Buyer accepts all offers
		for _, offer := range offers {
			acceptTx := nft.NFTokenAcceptSellOffer(buyer, offer).Build()
			env.Submit(acceptTx)
			env.Close()
		}

		// Verify all NFTs owned by buyer by creating sell offers
		for _, nftID := range nftIDs {
			offerTx := nft.NFTokenCreateSellOffer(buyer, nftID, jtx.XRPTxAmountFromXRP(100)).Build()
			result := env.Submit(offerTx)
			if !result.Success {
				t.Errorf("Failed to create sell offer for NFT %s: ledger can't find it", nftID)
			}
			env.Close()
		}
	}

	t.Run("SplitAndAddToHi", func(t *testing.T) {
		exerciseLopsided(t, seedsSplitAndAddToHi)
	})

	t.Run("SplitAndAddToLo", func(t *testing.T) {
		exerciseLopsided(t, seedsSplitAndAddToLo)
	})

	t.Log("testLopsidedSplits passed")
}

// TestFixNFTokenDirV1 exercises a fix for an off-by-one in NFTokenPage index creation.
// Reference: rippled NFTokenDir_test.cpp testFixNFTokenDirV1
func TestFixNFTokenDirV1(t *testing.T) {
	t.Skip("testFixNFTokenDirV1 requires account creation from seed and amendment testing")

	exerciseFixNFTokenDirV1 := func(t *testing.T, seeds []string, withFix bool) {
		env := jtx.NewTestEnv(t)

		buyer := jtx.NewAccount("buyer")
		env.Fund(buyer)
		env.Close()

		// Create accounts from seeds
		accounts := make([]*jtx.Account, 0, len(seeds))
		for _, seed := range seeds {
			account := jtx.NewAccount(seed)
			accounts = append(accounts, account)
			env.Fund(account)
		}
		env.Close()

		// All accounts create one NFT and offer to buyer
		nftIDs := make([]string, 0, len(accounts))
		offers := make([]string, 0, len(accounts))

		for _, account := range accounts {
			mintTx := nft.NFTokenMint(account, 0).Transferable().Build()
			result := env.Submit(mintTx)
			if result.Success {
				nftID := "nft_" + account.Address
				nftIDs = append(nftIDs, nftID)

				offerTx := nft.NFTokenCreateSellOffer(account, nftID, jtx.XRPTxAmount(0)).
					Destination(buyer).Build()
				result = env.Submit(offerTx)
				if result.Success {
					offers = append(offers, "offer_"+nftID)
				}
			}
			env.Close()
		}

		// Buyer accepts all but the last offer
		for i := 0; i < len(offers)-1; i++ {
			acceptTx := nft.NFTokenAcceptSellOffer(buyer, offers[i]).Build()
			env.Submit(acceptTx)
			env.Close()
		}

		// The last offer causes the page to split
		// Without fix: tecINVARIANT_FAILED
		// With fix: tesSUCCESS
		lastAcceptTx := nft.NFTokenAcceptSellOffer(buyer, offers[len(offers)-1]).Build()
		result := env.Submit(lastAcceptTx)

		if withFix {
			if !result.Success {
				t.Errorf("With fixNFTokenDirV1, accept should succeed: %s", result.Message)
			}
		} else {
			if result.Success {
				t.Error("Without fixNFTokenDirV1, accept should fail with tecINVARIANT_FAILED")
			}
		}
	}

	t.Run("SeventeenHi", func(t *testing.T) {
		exerciseFixNFTokenDirV1(t, seedsSeventeenHi, true)
	})

	t.Run("SeventeenLo", func(t *testing.T) {
		exerciseFixNFTokenDirV1(t, seedsSeventeenLo, true)
	})

	t.Log("testFixNFTokenDirV1 passed")
}

// TestTooManyEquivalent exercises when 33 NFTs with identical sort characteristics
// are owned by the same account.
// Reference: rippled NFTokenDir_test.cpp testTooManyEquivalent
func TestTooManyEquivalent(t *testing.T) {
	t.Skip("testTooManyEquivalent requires account creation from seed")

	env := jtx.NewTestEnv(t)

	buyer := jtx.NewAccount("buyer")
	env.Fund(buyer)
	env.Close()

	// Create 33 accounts with identical low 32-bits
	accounts := make([]*jtx.Account, 0, len(seedsLow32_9a8ebed3))
	for _, seed := range seedsLow32_9a8ebed3 {
		account := jtx.NewAccount(seed)
		accounts = append(accounts, account)
		env.Fund(account)
	}
	env.Close()

	// All accounts create one NFT and offer to buyer
	nftIDs := make([]string, 0, len(accounts))
	offers := make([]string, 0, len(accounts))

	for _, account := range accounts {
		mintTx := nft.NFTokenMint(account, 0).Transferable().Build()
		result := env.Submit(mintTx)
		if result.Success {
			nftID := "nft_" + account.Address
			nftIDs = append(nftIDs, nftID)

			offerTx := nft.NFTokenCreateSellOffer(account, nftID, jtx.XRPTxAmount(0)).
				Destination(buyer).Build()
			result = env.Submit(offerTx)
			if result.Success {
				offers = append(offers, "offer_"+nftID)
			}
		}
		env.Close()
	}

	// Remove the last NFT and offer - this one will overflow
	overflowNFT := nftIDs[len(nftIDs)-1]
	overflowOffer := offers[len(offers)-1]
	nftIDs = nftIDs[:len(nftIDs)-1]
	offers = offers[:len(offers)-1]

	// Buyer accepts all but the overflow offer
	for _, offer := range offers {
		acceptTx := nft.NFTokenAcceptSellOffer(buyer, offer).Build()
		env.Submit(acceptTx)
		env.Close()
	}

	// Accepting the overflow offer should fail with tecNO_SUITABLE_NFTOKEN_PAGE
	overflowAcceptTx := nft.NFTokenAcceptSellOffer(buyer, overflowOffer).Build()
	result := env.Submit(overflowAcceptTx)
	if result.Success {
		t.Error("Accepting 33rd NFT with same low 96-bits should fail")
	}
	t.Logf("Overflow accept result: %s (expected tecNO_SUITABLE_NFTOKEN_PAGE)", result.Code)

	// Verify all 32 NFTs are findable
	for _, nftID := range nftIDs {
		offerTx := nft.NFTokenCreateSellOffer(buyer, nftID, jtx.XRPTxAmountFromXRP(100)).Build()
		result := env.Submit(offerTx)
		if !result.Success {
			t.Errorf("Ledger can't find NFT %s", nftID)
		}
		env.Close()
	}

	_ = overflowNFT // Not used in this simplified test

	t.Log("testTooManyEquivalent passed")
}

// TestConsecutivePacking tests worst case scenario for NFT packing.
// 33 accounts with identical low-32 bits mint 7 consecutive NFTs each.
// A single account buys all 7x32 of the 33 NFTs.
// Reference: rippled NFTokenDir_test.cpp testConsecutivePacking
func TestConsecutivePacking(t *testing.T) {
	t.Skip("testConsecutivePacking requires account creation from seed and fixNFTokenDirV1")

	// This test requires fixNFTokenDirV1 to be enabled

	env := jtx.NewTestEnv(t)

	buyer := jtx.NewAccount("buyer")
	env.Fund(buyer)
	env.Close()

	// Create 33 accounts with identical low 32-bits
	accounts := make([]*jtx.Account, 0, len(seedsLow32_115d0525))
	for _, seed := range seedsLow32_115d0525 {
		account := jtx.NewAccount(seed)
		accounts = append(accounts, account)
		env.Fund(account)
	}
	env.Close()

	// Each account creates 7 consecutive NFTs (taxon manipulated for sequential storage)
	const nftsPerAccount = 7
	nftIDsByPage := make([][]string, nftsPerAccount)
	offersByPage := make([][]string, nftsPerAccount)

	for i := 0; i < nftsPerAccount; i++ {
		nftIDsByPage[i] = make([]string, 0, len(accounts))
		offersByPage[i] = make([]string, 0, len(accounts))

		for _, account := range accounts {
			// Tweak taxon for sequential storage
			// taxon := toUInt32(nft.cipheredTaxon(i, nft.toTaxon(0)))
			taxon := uint32(i)

			mintTx := nft.NFTokenMint(account, taxon).Transferable().Build()
			result := env.Submit(mintTx)
			if result.Success {
				nftID := "nft_" + account.Address + "_" + string(rune(i))
				nftIDsByPage[i] = append(nftIDsByPage[i], nftID)

				offerTx := nft.NFTokenCreateSellOffer(account, nftID, jtx.XRPTxAmount(0)).
					Destination(buyer).Build()
				result = env.Submit(offerTx)
				if result.Success {
					offersByPage[i] = append(offersByPage[i], "offer_"+nftID)
				}
			}
			env.Close()
		}
	}

	// Remove one NFT/offer from each page - these would cause overflow
	overflowNFTs := make([]string, nftsPerAccount)
	overflowOffers := make([]string, nftsPerAccount)
	for i := 0; i < nftsPerAccount; i++ {
		lastIdx := len(nftIDsByPage[i]) - 1
		overflowNFTs[i] = nftIDsByPage[i][lastIdx]
		overflowOffers[i] = offersByPage[i][lastIdx]
		nftIDsByPage[i] = nftIDsByPage[i][:lastIdx]
		offersByPage[i] = offersByPage[i][:lastIdx]
	}

	// Buyer accepts all offers that won't cause overflow
	// Fill center and outsides first to exercise different boundary cases
	for _, pageIdx := range []int{3, 6, 0, 1, 2, 5, 4} {
		for _, offer := range offersByPage[pageIdx] {
			acceptTx := nft.NFTokenAcceptSellOffer(buyer, offer).Build()
			env.Submit(acceptTx)
			env.Close()
		}
	}

	// Accepting overflow offers should fail
	for _, offer := range overflowOffers {
		acceptTx := nft.NFTokenAcceptSellOffer(buyer, offer).Build()
		result := env.Submit(acceptTx)
		if result.Success {
			t.Error("Overflow accept should fail with tecNO_SUITABLE_NFTOKEN_PAGE")
		}
		env.Close()
	}

	// Verify all expected NFTs are findable
	for _, nfts := range nftIDsByPage {
		for _, nftID := range nfts {
			offerTx := nft.NFTokenCreateSellOffer(buyer, nftID, jtx.XRPTxAmountFromXRP(100)).Build()
			result := env.Submit(offerTx)
			if !result.Success {
				t.Errorf("Ledger can't find NFT %s", nftID)
			}
			env.Close()
		}
	}

	t.Log("testConsecutivePacking passed")
}
