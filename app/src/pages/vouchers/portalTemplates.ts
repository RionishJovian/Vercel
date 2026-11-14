// The nine router login page templates. Ids match internal/provisioner/routeros/portal_templates.go; the server
// renders every one of them, this list only drives the picker and says which are Store add-ons.
export type TemplateGroup = "myanmar" | "commercial";
export interface PortalTemplate { id: string; group: TemplateGroup; name: string; nameMy: string; bg: string; surface: string; ink: string; accent: string; primary: string; tint: string }

export const PORTAL_TEMPLATES: PortalTemplate[] = [
  { id: "starlink_mm", group: "myanmar", name: "Starlink Community Hub", nameMy: "Starlink အသိုင်းအဝိုင်း ဟပ်", bg: "#070d19", surface: "#0f1a2e", ink: "#f1f5fb", accent: "#5cc8ff", primary: "#5cc8ff", tint: "#1b3a66" },
  { id: "cultural_mm", group: "myanmar", name: "Myanmar Cultural Heritage", nameMy: "မြန်မာ့ယဉ်ကျေးမှု အမွေအနှစ်", bg: "#2a0e12", surface: "#38141a", ink: "#fbf3e4", accent: "#d4a338", primary: "#d4a338", tint: "#7a1f2b" },
  { id: "mountain_mm", group: "myanmar", name: "Highland & Mountain WISP", nameMy: "တောင်ပေါ် WISP", bg: "#0a221e", surface: "#13332c", ink: "#ecf6f2", accent: "#6ee7b7", primary: "#34d399", tint: "#0f766e" },
  { id: "village_mm", group: "myanmar", name: "Village & Community Wi-Fi", nameMy: "ရွာနှင့် ရပ်ကွက် Wi-Fi", bg: "#fff1dc", surface: "#ffffff", ink: "#3b1f0e", accent: "#b45309", primary: "#c2562a", tint: "#f59e0b" },
  { id: "obsidian", group: "commercial", name: "Obsidian Telemetry", nameMy: "Obsidian Telemetry", bg: "#0b0e14", surface: "#121620", ink: "#f8fafc", accent: "#38bdf8", primary: "#38bdf8", tint: "#121620" },
  { id: "cafe", group: "commercial", name: "Warm Café & Bistro", nameMy: "ကော်ဖီဆိုင်နှင့် ဘစ်စထရို", bg: "#f3e6d3", surface: "#fffaf2", ink: "#2b1a12", accent: "#8a4b24", primary: "#6f3b1e", tint: "#c98b4b" },
  { id: "hotel", group: "commercial", name: "Boutique Hotel & Lounge", nameMy: "ဟိုတယ်နှင့် လောင်းချ်", bg: "#ece7dc", surface: "#fbfaf6", ink: "#1a1a18", accent: "#7a5c1f", primary: "#1f2a44", tint: "#b08d3c" },
  { id: "express", group: "commercial", name: "Express Retail & Transit", nameMy: "Express လက်လီနှင့် ခရီးသွား", bg: "#0a0a0a", surface: "#171717", ink: "#ffffff", accent: "#facc15", primary: "#facc15", tint: "#171717" },
  { id: "daylight", group: "commercial", name: "Clean Daylight", nameMy: "Clean Daylight", bg: "#f5f6f8", surface: "#ffffff", ink: "#0b1220", accent: "#0a58ca", primary: "#0a58ca", tint: "#0a58ca" },
];

/** Free for every account. The other seven are Store add-ons (the server decides; this only labels them). */
export const FREE_TEMPLATES = ["starlink_mm", "daylight"];
export const isPremiumTemplate = (id: string) => id !== "" && !FREE_TEMPLATES.includes(id) && PORTAL_TEMPLATES.some((t) => t.id === id);
/** A portal that never chose a template keeps its original look, which is the obsidian one. */
export const effectiveTemplate = (id: string) => id || "obsidian";
