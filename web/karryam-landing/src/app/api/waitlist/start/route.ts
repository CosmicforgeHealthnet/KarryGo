import { neon } from "@neondatabase/serverless";
import { NextResponse } from "next/server";
import { validateWaitlist } from "@/lib/schemas";

export async function POST(req: Request) {
  let body: unknown;
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "Invalid request." }, { status: 400 });
  }

  // Honeypot: real users never fill this hidden field
  if (typeof (body as { website?: unknown })?.website === "string" && (body as { website: string }).website) {
    return NextResponse.json({ ok: true });
  }

  const result = validateWaitlist(body);
  if (!result.ok) {
    return NextResponse.json({ error: result.error }, { status: 400 });
  }

  const databaseUrl = process.env.DATABASE_URL?.trim();
  if (!databaseUrl) {
    console.error("waitlist unavailable: DATABASE_URL is missing in this deployment");
    return NextResponse.json(
      { error: "Registration is temporarily unavailable. Please try again later." },
      { status: 503 },
    );
  }

  try {
    const sql = neon(databaseUrl);
    const { email, segment } = result.data;
    await sql`
      insert into waitlist (email, segment)
      values (${email}, ${segment})
      on conflict (email, segment) do nothing
    `;
    return NextResponse.json({ ok: true });
  } catch (err) {
    console.error("waitlist insert failed", err);
    return NextResponse.json({ error: "Something went wrong. Try again." }, { status: 500 });
  }
}
