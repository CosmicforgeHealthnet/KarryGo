"""Render and check the local design deliverables with installed Chrome."""
from pathlib import Path
import json
from playwright.sync_api import sync_playwright
from PIL import Image, ImageDraw
from pypdf import PdfReader

ROOT = Path(__file__).resolve().parent
URL = (ROOT / 'index.html').as_uri()
IDS = ['splash','welcome','catalogue','fulfilment','sign-in-phone','sign-in-email','code-phone','code-email','vendor-choice','signed-in','invalid-code','expired-code','verifying','network-error','access-unavailable','access-suspended']
results = {'artboard': [393,852], 'screens': [], 'responsive': [], 'prototype': []}
with sync_playwright() as p:
    browser = p.chromium.launch(executable_path=r'C:\Program Files\Google\Chrome\Application\chrome.exe', headless=True, args=['--allow-file-access-from-files'])
    page = browser.new_page(viewport={'width':393,'height':852}, device_scale_factor=1)
    errors = []
    page.on('pageerror', lambda e: errors.append(str(e)))
    for sid in IDS:
        page.goto(URL + '?mode=phone&screen=' + sid)
        page.evaluate('document.fonts.ready')
        page.locator('#' + sid + ' .phone').screenshot(path=str(ROOT/'screens'/(sid+'.png')))
        evidence = page.locator('#'+sid+' .phone').evaluate('''el => ({width:el.clientWidth,height:el.clientHeight,overflow:el.scrollWidth>el.clientWidth,missingImages:[...el.querySelectorAll('img')].filter(i=>!i.complete||i.naturalWidth===0).length, duplicateIds:[...el.querySelectorAll('[id]')].filter(n=>document.querySelectorAll('[id="'+n.id+'"]').length>1).map(n=>n.id), textOverflow:[...el.querySelectorAll('h2,p,label,button,.vendor-info small')].filter(n=>n.scrollWidth>n.clientWidth+1).map(n=>n.textContent.trim())})''')
        assert evidence['width']==393 and evidence['height']==852, (sid,evidence)
        assert not evidence['overflow'] and not evidence['missingImages'] and not evidence['textOverflow'] and not evidence['duplicateIds'], (sid,evidence)
        results['screens'].append({'id':sid,**evidence})
    for width,height in [(360,800),(375,812)]:
        page.set_viewport_size({'width':width,'height':height})
        for sid in IDS:
            page.goto(URL + '?mode=phone&screen=' + sid)
            evidence = page.locator('#'+sid+' .phone').evaluate('''el=>({width:el.clientWidth,height:el.clientHeight,overflow:el.scrollWidth>el.clientWidth,footerVisible:[...el.querySelectorAll('.auth-footer,.footer')].every(n=>n.getBoundingClientRect().bottom<=el.getBoundingClientRect().bottom-20)})''')
            assert evidence['width']==width and evidence['height']==height and not evidence['overflow'] and evidence['footerVisible'], (sid,width,evidence)
        results['responsive'].append({'viewport':[width,height],'screensChecked':len(IDS)})
    page.set_viewport_size({'width':393,'height':852})
    page.goto(URL+'?mode=prototype&screen=sign-in-phone')
    page.locator('#phone').fill('+2348012345678')
    page.locator('#sign-in-phone .auth-footer .primary').click()
    assert page.locator('#code-phone').is_visible()
    assert not page.locator('#code-phone .verify-button').is_enabled()
    page.locator('#code-phone .otp-input').fill('123456')
    assert page.locator('#code-phone .verify-button').is_enabled()
    page.locator('#code-phone .verify-button').click()
    page.wait_for_selector('#vendor-choice:not([hidden])')
    page.locator('#vendor-choice [data-vendor]').nth(1).click()
    assert page.locator('#vendor-choice [data-vendor]').nth(1).get_attribute('aria-checked')=='true'
    page.locator('#vendor-choice .auth-footer .primary').click()
    assert page.locator('#signed-in').is_visible()
    results['prototype'].append('Phone → OTP → conditional vendor choice → signed in')
    page.goto(URL+'?mode=prototype&screen=sign-in-email')
    page.locator('#email').fill('owner@example.com')
    page.locator('#sign-in-email .auth-footer .primary').click()
    assert page.locator('#code-email').is_visible()
    page.locator('#code-email [data-resend]').click()
    assert page.locator('#code-email').is_visible()
    results['prototype'].append('Email → OTP → resend preview')
    page.goto(URL+'?mode=prototype&screen=welcome')
    page.locator('#welcome [data-go="catalogue"]').click()
    page.locator('#catalogue [data-go="fulfilment"]').click()
    page.locator('#fulfilment .footer .primary').click()
    assert page.locator('#sign-in-phone').is_visible()
    results['prototype'].append('Three onboarding slides → sign in')
    page.goto(URL+'?mode=prototype&screen=welcome')
    page.locator('#welcome .nav [data-go="sign-in-phone"]').click()
    assert page.locator('#sign-in-phone').is_visible()
    results['prototype'].append('Skip onboarding → sign in')
    page.goto(URL)
    page.evaluate('document.fonts.ready')
    page.pdf(path=str(ROOT/'Karryam-Vendor-Onboarding-Auth.pdf'), width='393px', height='852px', print_background=True, margin={'top':'0','right':'0','bottom':'0','left':'0'}, prefer_css_page_size=True)
    pdf=PdfReader(ROOT/'Karryam-Vendor-Onboarding-Auth.pdf')
    assert len(pdf.pages)==len(IDS), len(pdf.pages)
    assert all((p.extract_text() or '').strip() for p in pdf.pages)
    results['pdfPages']=len(pdf.pages)
    assert not errors, errors
    results['browserErrors']=errors
    browser.close()

# Contact sheet combines original screen exports without changing their designs.
thumb_w,thumb_h=262,568
cols=3;rows=3;gap=24;label_h=32
sheet=Image.new('RGB',(cols*thumb_w+(cols+1)*gap,rows*(thumb_h+label_h)+(rows+1)*gap),'#eef1f5')
draw=ImageDraw.Draw(sheet)
for idx,sid in enumerate(IDS[:9]):
    image=Image.open(ROOT/'screens'/(sid+'.png')).convert('RGB').resize((thumb_w,thumb_h),Image.Resampling.LANCZOS)
    x=gap+(idx%cols)*(thumb_w+gap);y=gap+(idx//cols)*(thumb_h+label_h+gap)
    draw.text((x,y+6),f'{idx+1:02d}  {sid.replace("-"," ")}',fill='#0B1F44')
    sheet.paste(image,(x,y+label_h))
sheet.save(ROOT/'screens'/'overview.png')
(ROOT/'validation.json').write_text(json.dumps(results,indent=2),encoding='utf-8')
print(json.dumps({'screenCount':len(results['screens']),'responsive':results['responsive'],'prototype':results['prototype'],'browserErrors':errors},indent=2))
