import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import { MagicDude } from "./MagicDude";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
let answers: Response[], sent: Array<{ question: string; history: Array<{ role: string; content: string }> }>;

beforeEach(() => {
  localStorage.setItem("mm2_lang", "en"); answers = []; sent = [];
  Element.prototype.scrollTo = vi.fn();
  vi.stubGlobal("fetch", vi.fn(async (_url: string, init?: RequestInit) => { sent.push(JSON.parse(String(init?.body))); return answers.shift()!; }));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const ask = async (q: string, count: number) => {
  fireEvent.change(screen.getByLabelText("What can I help with?"), { target: { value: q } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(sent).toHaveLength(count));
  await waitFor(() => expect((screen.getByLabelText("What can I help with?") as HTMLTextAreaElement).disabled).toBe(false));
};
const open = () => { render(<I18nProvider><MagicDude /></I18nProvider>); fireEvent.click(screen.getByRole("button", { name: "Magic Dude" })); };

describe("Magic Dude", () => {
  it("keeps a failed exchange out of the conversation it sends", async () => {
    answers = [json(200, { answer: "first answer" }), json(502, { error: "down" }), json(200, { answer: "third answer" })];
    open();
    await ask("first", 1);
    await ask("second", 2);
    await screen.findByText("No answer right now. Please try again. Nothing was changed.");
    await ask("third", 3);
    expect(sent[2].history).toEqual([{ role: "user", content: "first" }, { role: "assistant", content: "first answer" }]);
    await screen.findByText("third answer");
  });
  it("shortens a very long earlier answer so the next question still goes", async () => {
    answers = [json(200, { answer: "မ".repeat(5000) }), json(200, { answer: "ok" })];
    open();
    await ask("first", 1);
    await ask("second", 2);
    expect(sent[1].history[1].content).toHaveLength(4000);
  });
  it("says when the usage limit is full", async () => {
    answers = [json(429, { error: "limit", code: "MAGIC_DUDE_DAILY_LIMIT" })];
    open();
    await ask("first", 1);
    expect((await screen.findAllByText("The AI usage limit is full. Try again later.")).length).toBeGreaterThan(0);
  });
  it("does not promise voucher answers", () => {
    open();
    expect(screen.getByText(/Ask about your devices/).textContent).not.toMatch(/voucher/i);
  });
});
