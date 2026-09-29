// Code generated from index-feed-map.json; DO NOT EDIT BY HAND.
// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

// indexFeedEntry is one row of the committed index table (Phase 1 of the
// datafeed plan): the join of the live scrip-master-equity.csv index rows to
// the vendor's index lists, on a normalised name. See feed_index_table_test.go,
// which rebuilds this file from index-feed-map.json and asserts a match.
type indexFeedEntry struct {
	feedSegment string // e.g. "nse_cm"
	feedSymbol  string // the name the feed subscribes this index by
	name        string // our own display name, carried for diagnostics only
}

// indexFeedMap holds 81 unambiguous entries from index-feed-map.json's
// "indices" array. See indexFeedAmbiguous and indexFeedAbsent for the rest of
// the source's scripKeys, which are deliberately not here.
var indexFeedMap = map[string]indexFeedEntry{
	"BSE@19000": {feedSegment: "bse_cm", feedSymbol: "SENSEX", name: "SENSEX"},
	"BSE@19001": {feedSegment: "bse_cm", feedSymbol: "BSEPSU", name: "BSEPSU"},
	"BSE@19002": {feedSegment: "bse_cm", feedSymbol: "BSE100", name: "BSE100"},
	"BSE@19003": {feedSegment: "bse_cm", feedSymbol: "BSE200", name: "BSE200"},
	"BSE@19004": {feedSegment: "bse_cm", feedSymbol: "BSE500", name: "BSE500"},
	"BSE@19005": {feedSegment: "bse_cm", feedSymbol: "BSE IT", name: "BSE IT"},
	"BSE@19006": {feedSegment: "bse_cm", feedSymbol: "BSEFMC", name: "BSEFMC"},
	"BSE@19007": {feedSegment: "bse_cm", feedSymbol: "BSE CG", name: "BSE CG"},
	"BSE@19008": {feedSegment: "bse_cm", feedSymbol: "BSE CD", name: "BSE CD"},
	"BSE@19009": {feedSegment: "bse_cm", feedSymbol: "BSE HC", name: "BSE HC"},
	"BSE@19011": {feedSegment: "bse_cm", feedSymbol: "TECK", name: "TECK"},
	"BSE@19012": {feedSegment: "bse_cm", feedSymbol: "BANKEX", name: "BANKEX"},
	"BSE@19013": {feedSegment: "bse_cm", feedSymbol: "AUTO", name: "AUTO"},
	"BSE@19014": {feedSegment: "bse_cm", feedSymbol: "METAL", name: "METAL"},
	"BSE@19015": {feedSegment: "bse_cm", feedSymbol: "CPSE", name: "CPSE"},
	"BSE@19016": {feedSegment: "bse_cm", feedSymbol: "MIDCAP", name: "MIDCAP"},
	"BSE@19017": {feedSegment: "bse_cm", feedSymbol: "SMLCAP", name: "SMLCAP"},
	"BSE@19019": {feedSegment: "bse_cm", feedSymbol: "DOL100", name: "DOL100"},
	"BSE@19020": {feedSegment: "bse_cm", feedSymbol: "DOL200", name: "DOL200"},
	"BSE@19051": {feedSegment: "bse_cm", feedSymbol: "OILGAS", name: "OILGAS"},
	"BSE@19052": {feedSegment: "bse_cm", feedSymbol: "POWER", name: "POWER"},
	"BSE@19053": {feedSegment: "bse_cm", feedSymbol: "REALTY", name: "REALTY"},
	"BSE@19054": {feedSegment: "bse_cm", feedSymbol: "BSEIPO", name: "BSEIPO"},
	"BSE@19059": {feedSegment: "bse_cm", feedSymbol: "SMEIPO", name: "SMEIPO"},
	"BSE@19060": {feedSegment: "bse_cm", feedSymbol: "INFRA", name: "INFRA"},
	"BSE@19089": {feedSegment: "bse_cm", feedSymbol: "LMI250", name: "LMI250"},
	"BSE@39":    {feedSegment: "bse_cm", feedSymbol: "ENERGY", name: "ENERGY"},
	"BSE@40":    {feedSegment: "bse_cm", feedSymbol: "FINSER", name: "FINSER"},
	"BSE@41":    {feedSegment: "bse_cm", feedSymbol: "INDSTR", name: "INDSTR"},
	"BSE@42":    {feedSegment: "bse_cm", feedSymbol: "LRGCAP", name: "LRGCAP"},
	"BSE@43":    {feedSegment: "bse_cm", feedSymbol: "MIDSEL", name: "MIDSEL"},
	"BSE@44":    {feedSegment: "bse_cm", feedSymbol: "SMLSEL", name: "SMLSEL"},
	"BSE@45":    {feedSegment: "bse_cm", feedSymbol: "TELCOM", name: "TELCOM"},
	"BSE@47":    {feedSegment: "bse_cm", feedSymbol: "SNSX50", name: "SNSX50"},
	"BSE@48":    {feedSegment: "bse_cm", feedSymbol: "SNXT50", name: "SNXT50"},
	"BSE@55":    {feedSegment: "bse_cm", feedSymbol: "MID150", name: "MID150"},
	"BSE@58":    {feedSegment: "bse_cm", feedSymbol: "MSL400", name: "MSL400"},
	"NSE@25998": {feedSegment: "nse_cm", feedSymbol: "Nifty 500", name: "NIFTY 500"},
	"NSE@26000": {feedSegment: "nse_cm", feedSymbol: "Nifty 50", name: "NIFTY 50"},
	"NSE@26001": {feedSegment: "nse_cm", feedSymbol: "Nifty GrowSect 15", name: "NIFTY GROWSECT 15"},
	"NSE@26008": {feedSegment: "nse_cm", feedSymbol: "Nifty IT", name: "NIFTY IT"},
	"NSE@26009": {feedSegment: "nse_cm", feedSymbol: "Nifty Bank", name: "NIFTY BANK"},
	"NSE@26012": {feedSegment: "nse_cm", feedSymbol: "Nifty 100", name: "NIFTY 100"},
	"NSE@26013": {feedSegment: "nse_cm", feedSymbol: "Nifty Next 50", name: "NIFTY NEXT 50"},
	"NSE@26014": {feedSegment: "nse_cm", feedSymbol: "Nifty Midcap 50", name: "NIFTY MIDCAP 50"},
	"NSE@26017": {feedSegment: "nse_cm", feedSymbol: "India VIX", name: "INDIA VIX"},
	"NSE@26018": {feedSegment: "nse_cm", feedSymbol: "Nifty Pharma", name: "NIFTY PHARMA"},
	"NSE@26019": {feedSegment: "nse_cm", feedSymbol: "Nifty Infra", name: "NIFTY INFRA"},
	"NSE@26021": {feedSegment: "nse_cm", feedSymbol: "Nifty Realty", name: "NIFTY REALTY"},
	"NSE@26022": {feedSegment: "nse_cm", feedSymbol: "Nifty MNC", name: "NIFTY MNC"},
	"NSE@26024": {feedSegment: "nse_cm", feedSymbol: "Nifty PSE", name: "NIFTY PSE"},
	"NSE@26026": {feedSegment: "nse_cm", feedSymbol: "Nifty Serv Sector", name: "NIFTY SERV SECTOR"},
	"NSE@26033": {feedSegment: "nse_cm", feedSymbol: "Nifty Auto", name: "NIFTY AUTO"},
	"NSE@26035": {feedSegment: "nse_cm", feedSymbol: "Nifty Consumption", name: "NIFTY CONSUMPTION"},
	"NSE@26036": {feedSegment: "nse_cm", feedSymbol: "Nifty 200", name: "NIFTY 200"},
	"NSE@26037": {feedSegment: "nse_cm", feedSymbol: "Nifty Fin Service", name: "NIFTY FIN SERVICE"},
	"NSE@26038": {feedSegment: "nse_cm", feedSymbol: "Nifty50 Div Point", name: "NIFTY50 DIV POINT"},
	"NSE@26041": {feedSegment: "nse_cm", feedSymbol: "Nifty CPSE", name: "NIFTY CPSE"},
	"NSE@26042": {feedSegment: "nse_cm", feedSymbol: "Nifty50 PR 1x Inv", name: "NIFTY50 PR 1X INV"},
	"NSE@26043": {feedSegment: "nse_cm", feedSymbol: "Nifty50 TR 2x Lev", name: "NIFTY50 TR 2X LEV"},
	"NSE@26048": {feedSegment: "nse_cm", feedSymbol: "NIFTY100 Qualty30", name: "NIFTY100 QUALTY30"},
	"NSE@26049": {feedSegment: "nse_cm", feedSymbol: "Nifty GS 8 13Yr", name: "NIFTY GS 8 13YR"},
	"NSE@26050": {feedSegment: "nse_cm", feedSymbol: "Nifty GS 10Yr", name: "NIFTY GS 10YR"},
	"NSE@26051": {feedSegment: "nse_cm", feedSymbol: "Nifty GS 10Yr Cln", name: "NIFTY GS 10YR CLN"},
	"NSE@26052": {feedSegment: "nse_cm", feedSymbol: "Nifty GS 4 8Yr", name: "NIFTY GS 4 8YR"},
	"NSE@26053": {feedSegment: "nse_cm", feedSymbol: "Nifty GS 11 15Yr", name: "NIFTY GS 11 15YR"},
	"NSE@26054": {feedSegment: "nse_cm", feedSymbol: "Nifty GS 15YrPlus", name: "NIFTY GS 15YRPLUS"},
	"NSE@26055": {feedSegment: "nse_cm", feedSymbol: "Nifty GS Compsite", name: "NIFTY GS COMPSITE"},
	"NSE@26056": {feedSegment: "nse_cm", feedSymbol: "NIFTY50 EQL Wgt", name: "NIFTY50 EQL WGT"},
	"NSE@26057": {feedSegment: "nse_cm", feedSymbol: "NIFTY100 EQL Wgt", name: "NIFTY100 EQL WGT"},
	"NSE@26058": {feedSegment: "nse_cm", feedSymbol: "NIFTY100 LowVol30", name: "NIFTY100 LOWVOL30"},
	"NSE@26059": {feedSegment: "nse_cm", feedSymbol: "NIFTY Alpha 50", name: "NIFTY ALPHA 50"},
	"NSE@26070": {feedSegment: "nse_cm", feedSymbol: "Nifty50 Value 20", name: "NIFTY50 VALUE 20"},
	"NSE@26074": {feedSegment: "nse_cm", feedSymbol: "NIFTY MID SELECT", name: "NIFTY MID SELECT"},
	"NSE@26076": {feedSegment: "nse_cm", feedSymbol: "Nifty Pvt Bank", name: "NIFTY PVT BANK"},
	"NSE@26085": {feedSegment: "nse_cm", feedSymbol: "Nifty Media", name: "NIFTY MEDIA"},
	"NSE@26090": {feedSegment: "nse_cm", feedSymbol: "NIFTY MIDCAP 150", name: "NIFTY MIDCAP 150"},
	"NSE@26091": {feedSegment: "nse_cm", feedSymbol: "NIFTY SMLCAP 50", name: "NIFTY SMLCAP 50"},
	"NSE@26092": {feedSegment: "nse_cm", feedSymbol: "NIFTY SMLCAP 250", name: "NIFTY SMLCAP 250"},
	"NSE@26093": {feedSegment: "nse_cm", feedSymbol: "NIFTY MIDSML 400", name: "NIFTY MIDSML 400"},
	"NSE@26094": {feedSegment: "nse_cm", feedSymbol: "NIFTY200 QUALTY30", name: "NIFTY200 QUALTY30"},
}

// indexFeedAmbiguous lists every scripKey the scrip master genuinely shares
// between two different indices (index-feed-map.json's "ambiguous" array: one
// token, two index names -- not a bad join, a fact about the scrip master).
// index-feed-map.json's own note explains why these are excluded from
// indexFeedMap rather than resolved by picking one: "subscribing to one must
// fail with a clear error rather than silently streaming whichever index won
// a tie-break." The prices of either candidate look entirely plausible, so a
// silent wrong pick is the worst failure mode here -- translateIndexKey
// names both candidates in the error instead, so the caller picks.
var indexFeedAmbiguous = map[string][]string{
	"NSE@26002": {"Nifty FMCG", "Nifty50 PR 2x Lev"},
	"NSE@26020": {"Nifty Energy", "Nifty PSU Bank"},
	"NSE@26034": {"Nifty Div Opps 50", "Nifty Metal"},
	"NSE@26040": {"Nifty Commodities", "Nifty100 Liq 15"},
	"NSE@26044": {"NIFTY MIDCAP 100", "Nifty50 TR 1x Inv"},
	"NSE@26046": {"NIFTY SMLCAP 100", "Nifty Mid Liq 15"},
}

// indexFeedAbsent lists every index scripKey the source join never produced a
// feed mapping for at all (absent from both of index-feed-map.json's arrays)
// -- the feed does not publish it, or the nearest candidate is not a safe
// match (DOLLEX 30) -- with the reason a caller sees in the error. These are
// supplied directly (datafeed plan, Phase 1 table), not derived from the
// source file, since by definition the source file never mentions them.
var indexFeedAbsent = map[string]string{
	"NSE@26016": "HANGSENG BEES-NAV is an ETF NAV pseudo-index, not traded on the feed",
	"BSE@19058": "S&P BSE CARBONEX is not published by the feed",
	"BSE@19081": "S&P BSE GREENEX is not published by the feed",
	"BSE@19018": "S&P BSE DOLLEX 30 has no feed counterpart (the feed has DOL300, DOL100, DOL200, not DOL30) and guessing one would stream the wrong index",
}
