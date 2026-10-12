# Design validation

Validated 2026-10-07 with headless installed Chrome and Playwright.

- All 16 screens exported at exactly **393 × 852 px** (1×).
- All 16 screens checked at **360 × 800** and **375 × 812**: correct viewport dimensions, no horizontal overflow, footer actions visible.
- All local images resolved; no duplicate element IDs or horizontal text overflow at the primary artboard size.
- Browser visual inspection: splash, merchant onboarding, Food/Shop slide, fulfilment illustration, sign-in, OTP and final nine-screen overview. Corrected sign-in action positioning and the smaller-phone splash route width.
- Phone sign-in → code entry → verification state → vendor choice → signed-in preview passed.
- Email sign-in and resend preview passed.
- Onboarding progression and Skip passed.
- Code action disabled until six digits are entered; second vendor selection updates its selected state.
- No browser JavaScript errors. `node --check design.js` passed.
- PDF checked for exactly 16 phone-sized pages with no empty pages.
- Original logo and existing imagery copied intact; exact official brand palette and local Inter fonts used.

`validation.json` contains the screen/responsive/interaction evidence. This validates a design prototype, not live backend behaviour. No Flutter widget implementation or staging authentication test is claimed. Full accessibility testing with a screen reader, actual OS autofill/keyboard behaviour and device testing belong to the Flutter implementation stage.
