import { csvCell, validSummary } from "./RevenuePayments";

describe("payment summary", () => {
  const good = { today: 1, last7: 2, last30: 3, all_time: 4, total: 5, approved: 3, pending: 1, rejected: 1, days: [], plans: [] };
  it("accepts a complete summary", () => { expect(validSummary(good)).not.toBeNull(); });
  it("rejects a partial or negative one instead of showing a wrong total", () => {
    expect(validSummary({ ...good, today: undefined })).toBeNull();
    expect(validSummary({ ...good, all_time: -1 })).toBeNull();
    expect(validSummary({ ...good, days: null })).toBeNull();
  });
});

describe("csv export", () => {
  it("neutralises spreadsheet formulas", () => { for (const v of ["=1+1", "+cmd", "-2", "@SUM(A1)"]) expect(csvCell(v)).toBe(`"'${v}"`); });
  it("quotes embedded quotes and leaves normal text alone", () => { expect(csvCell('say "hi"')).toBe('"say ""hi"""'); expect(csvCell("KBZ-123")).toBe('"KBZ-123"'); });
});
