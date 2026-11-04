import { beforeEach, afterEach, it, expect, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { ConnectionCheck } from "./ConnectionCheck";

let body: Record<string,unknown>, status: number;
let requests: Array<{url:string;method:string}>;
beforeEach(()=>{
 localStorage.setItem("mm2_lang","en"); status=200; requests=[];
 body={handshake_status:"no_recent_handshake",tunnel_address:"10.77.0.16/32",last_handshake_at:null,connection:{management:"missing",setup:"expired"}};
 vi.stubGlobal("fetch",vi.fn(async(url:string,init?:RequestInit)=>{requests.push({url:String(url),method:init?.method??"GET"});return new Response(JSON.stringify(body),{status,headers:{"content-type":"application/json"}});}));
});
afterEach(()=>{cleanup();vi.unstubAllGlobals();});
const open=()=>{render(<I18nProvider><ConnectionCheck id="router-a" /></I18nProvider>);fireEvent.click(screen.getByRole("button",{name:"Check Magic Hub"}));};

it("explains expired partial setup using a read without generating another script",async()=>{
 open();await screen.findByText(/Setup expired\. Repair works only/);
 expect(screen.getByText("No recent tunnel handshake")).toBeTruthy();expect(screen.getByText("No saved management access")).toBeTruthy();
 expect(requests).toEqual([{url:"/api/v1/vps/peers/router-a/live",method:"GET"}]);
});
it("does not treat a handshake and saved credentials as management verification",async()=>{
 body={...body,handshake_status:"recent_handshake",connection:{management:"saved",setup:"not_required"}};
 open();await screen.findByText("Saved access; check the connection to verify");
 expect(screen.getByText("Recent tunnel handshake")).toBeTruthy();expect(screen.queryByText(/Setup expired/)).toBeNull();
 expect(screen.queryByText(/Connected\. Router management is ready/)).toBeNull();
});
it("keeps absent diagnostics unknown instead of claiming setup or management is missing",async()=>{
 delete body.connection;open();await screen.findByText("No recent tunnel handshake");
 expect(screen.getAllByText("Status unavailable")).toHaveLength(2);expect(screen.queryByText("No saved management access")).toBeNull();
});
it("distinguishes a pending repair from expired original installation material",async()=>{
 body.connection={management:"missing",setup:"repair_pending"};open();
 await screen.findByText("Repair is awaiting verification. The original tunnel must be present on the router.");
 expect(screen.queryByText(/Setup expired/)).toBeNull();
});
it("clears stale diagnostic conclusions when a later status read fails",async()=>{
 open();await screen.findByText(/Setup expired\. Repair works only/);
 status=502;body={code:"hub_lookup_failed"};fireEvent.click(screen.getByRole("button",{name:"Check Magic Hub"}));
 await screen.findByText(/The lookup could not finish/);
 expect(screen.queryByText(/Setup expired/)).toBeNull();
 expect(requests.every(r=>r.method==="GET")).toBe(true);
});
