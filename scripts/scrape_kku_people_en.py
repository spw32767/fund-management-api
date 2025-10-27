#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import os, sys, time, re, json
from urllib.parse import urljoin, urlparse
from bs4 import BeautifulSoup

from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.chrome.options import Options as ChromeOptions
from selenium.webdriver.edge.options import Options as EdgeOptions
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC

BASE = "https://computing.kku.ac.th"
LANG_PREFIXES = ("/en", "/th")  # รองรับหน้า EN/TH

TAB_LABELS = ("ทั้งหมด", "All", "ผู้บริหาร", "สายวิชาการ", "สายสนับสนุน")

# ========== I/O ==========
try:
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

def eprint(*a, **k): print(*a, file=sys.stderr, **k)

# ========== WebDriver ==========
def _chrome_driver(headless=True):
    opts = ChromeOptions()
    if headless: opts.add_argument("--headless=new")
    opts.add_argument("--no-sandbox")
    opts.add_argument("--disable-dev-shm-usage")
    opts.add_argument("--disable-gpu")
    opts.add_argument("--window-size=1400,1000")
    opts.add_argument("--lang=th-TH,th;q=0.9,en-US;q=0.8,en;q=0.7")
    # ลดร่องรอย automation
    opts.add_experimental_option("excludeSwitches", ["enable-automation"])
    opts.add_experimental_option("useAutomationExtension", False)
    opts.add_argument("--disable-blink-features=AutomationControlled")
    opts.add_argument(
        "user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
        "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
    )
    chrome_bin = os.environ.get("CHROME_BINARY", "").strip()
    if chrome_bin:
        if not os.path.isfile(chrome_bin):
            raise FileNotFoundError(f"CHROME_BINARY not found: {chrome_bin}")
        opts.binary_location = chrome_bin
    drv = webdriver.Chrome(options=opts)
    try:
        drv.execute_cdp_cmd(
            "Page.addScriptToEvaluateOnNewDocument",
            {"source": "Object.defineProperty(navigator, 'webdriver', {get: () => undefined});"}
        )
    except Exception:
        pass
    return drv

def _edge_driver(headless=True):
    opts = EdgeOptions()
    if headless: opts.add_argument("--headless=new")
    opts.add_argument("--no-sandbox")
    opts.add_argument("--disable-dev-shm-usage")
    opts.add_argument("--disable-gpu")
    opts.add_argument("--window-size=1400,1000")
    opts.add_argument("--lang=th-TH,th;q=0.9,en-US;q=0.8,en;q=0.7")
    return webdriver.Edge(options=opts)

def make_driver():
    headless_env = os.environ.get("HEADLESS", "").strip().lower()
    headless = not (headless_env in ("0","false","no"))
    prefer = os.environ.get("BROWSER","auto").lower()
    if prefer in ("chrome","auto"):
        try:
            return _chrome_driver(headless=headless)
        except Exception:
            if prefer == "chrome": raise
    return _edge_driver(headless=headless)

def wait_ready(driver, timeout=30):
    WebDriverWait(driver, timeout).until(
        lambda d: d.execute_script("return document.readyState") == "complete"
    )

# ========== Scroll / Click ==========
def _scroll_window_to_bottom(driver, max_pass=30, sleep=0.25):
    last = 0
    for _ in range(max_pass):
        driver.execute_script("window.scrollTo(0, document.body.scrollHeight);")
        time.sleep(sleep)
        h = driver.execute_script("return document.body.scrollHeight;")
        if h == last: break
        last = h

def _largest_scrollable_container(driver):
    js = """
    const els = Array.from(document.querySelectorAll('main,section,div,ul,ol,article'));
    const sc = els.filter(e => e.scrollHeight - e.clientHeight > 40 && getComputedStyle(e).overflowY !== 'hidden');
    sc.sort((a,b)=>(b.scrollHeight-b.clientHeight)-(a.scrollHeight-b.clientHeight));
    return sc[0] || null;
    """
    try: return driver.execute_script(js)
    except Exception: return None

def _click_texty_buttons(driver, scope, texts, max_round=60):
    for _ in range(max_round):
        clicked = False
        try:
            xp = ".//*[self::a or self::button or @role='button'][" + \
                 " or ".join([f"contains(translate(normalize-space(.),'ABCDEFGHIJKLMNOPQRSTUVWXYZ','abcdefghijklmnopqrstuvwxyz'), '{t.lower()}')" for t in texts]) + \
                 "]"
            els = scope.find_elements(By.XPATH, xp)
        except Exception:
            els = []
        for el in els:
            try:
                driver.execute_script("arguments[0].scrollIntoView({block:'center'});", el)
                time.sleep(0.1)
                if el.is_displayed() and el.is_enabled():
                    try: el.click()
                    except Exception: driver.execute_script("arguments[0].click();", el)
                    time.sleep(0.6)
                    clicked = True
                    break
            except Exception:
                pass
        if not clicked: break

def _load_everything_in_view(driver):
    # คลิกปุ่มโหลด/เพิ่มเติม/next จนหมด + เลื่อน
    _click_texty_buttons(
        driver, driver,
        ["โหลดเพิ่ม","โหลดเพิ่มเติม","ดูเพิ่มเติม","เพิ่มเติม",
         "Load more","More","Show more","Next","ถัดไป"], max_round=80
    )
    sc = _largest_scrollable_container(driver)
    if sc:
        prev = -1
        for _ in range(40):
            try: driver.execute_script("arguments[0].scrollTop = arguments[0].scrollHeight;", sc)
            except Exception: pass
            time.sleep(0.3)
            try: cur = driver.execute_script("return arguments[0].scrollHeight;", sc)
            except Exception: cur = prev
            if cur == prev: break
            prev = cur
    _scroll_window_to_bottom(driver, max_pass=20, sleep=0.25)

# ========== URL helpers ==========
STATIC_EXT = re.compile(r"\.(?:css|js|png|jpe?g|gif|svg|webp|woff2?|ttf|ico|pdf)(?:\?|#|$)", re.I)
ALLOWED_SEGMENT_RE = re.compile(r"^[A-Za-z0-9_.\-ก-๙]+$")  # อนุญาตไทย/ขีดกลาง/ขีดล่าง/จุด

def _to_abs_url(href: str) -> str:
    if not href: return ""
    href = href.strip()
    if href.startswith("/"): return urljoin(BASE, href)
    return href

def _norm_abs(url: str) -> str:
    if not url: return ""
    if not url.startswith("http"): url = _to_abs_url(url)
    u = urlparse(url)
    return f"{u.scheme}://{u.netloc}{u.path}".rstrip("/")

def _strip_lang_prefix(path: str) -> str:
    for p in LANG_PREFIXES:
        if path == p: return "/"
        if path.startswith(p + "/"): return path[len(p):]
    return path

def _is_profile_abs_url(abs_url: str) -> bool:
    """
    ยอมรับเฉพาะโปรไฟล์แบบ root 1 segment ที่มีจุด:
    /firstname.lastname  (รองรับ /en/... และ /th/...)
    """
    if not abs_url or not abs_url.startswith(BASE): return False
    p = urlparse(abs_url); path = p.path or ""
    if not path or path == "/": return False
    if STATIC_EXT.search(path): return False
    eff = _strip_lang_prefix(path)
    segs = [s for s in eff.split("/") if s]
    if len(segs) == 1 and "." in segs[0] and ALLOWED_SEGMENT_RE.match(segs[0]):
        return True
    return False

# ========== Collectors ==========
def _collect_candidates_via_js(driver):
    js = r"""
    const uniq = new Set();
    const pick = v => { if (typeof v === 'string' && v.startsWith('/')) uniq.add(v); };
    document.querySelectorAll('a[href]').forEach(a => pick(a.getAttribute('href')||''));
    document.querySelectorAll('[to]').forEach(el => pick(el.getAttribute('to')||''));
    document.querySelectorAll('[onclick]').forEach(el => {
      const v = el.getAttribute('onclick') || '';
      const m = v.match(/location\.href\s*=\s*['"]([^'"]+)['"]/);
      if (m) pick(m[1]);
    });
    return Array.from(uniq);
    """
    try: rels = driver.execute_script(js) or []
    except Exception: rels = []
    return {_norm_abs(_to_abs_url(r)) for r in rels}

def _fallback_extract_from_html(html: str):
    links = set()
    if not html: return links
    for m in re.finditer(r'href=["\'](/[^"\']{1,200})["\']', html):
        links.add(_norm_abs(urljoin(BASE, m.group(1))))
    for m in re.finditer(r'"(?:path|to|link)"\s*:\s*"(/[^"\\]{1,200})"', html):
        links.add(_norm_abs(urljoin(BASE, m.group(1))))
    return links

def _extract_from_nuxt(driver):
    links = set()
    try:
        nuxt = driver.execute_script("return window.__NUXT__ || null;")
    except Exception:
        nuxt = None
    def walk(o):
        if isinstance(o, dict):
            for _, v in o.items():
                if isinstance(v, str) and v.startswith("/"):
                    links.add(_norm_abs(urljoin(BASE, v)))
                walk(v)
        elif isinstance(o, list):
            for it in o: walk(it)
    if nuxt: walk(nuxt)
    return links

# ========== Tabs ==========
def _click_tab(driver, label) -> bool:
    want = label.strip().lower()
    tabs = driver.find_elements(By.CSS_SELECTOR, "[role='tab'], .v-tab, button, a, li")
    target = None
    for t in tabs:
        txt = (t.text or "").strip().lower() or (t.get_attribute("aria-label") or "").strip().lower()
        if want and txt and want in txt:
            target = t; break
    if target is None:
        xp = f"//*[self::a or self::button or self::div or self::span or self::li][contains(normalize-space(.), '{label}')]"
        try:
            target = WebDriverWait(driver, 6).until(EC.element_to_be_clickable((By.XPATH, xp)))
        except Exception:
            return False
    try:
        driver.execute_script("arguments[0].scrollIntoView({block:'center'});", target)
        time.sleep(0.1)
        target.click()
    except Exception:
        try: driver.execute_script("arguments[0].click();", target)
        except Exception: return False
    time.sleep(0.6)
    return True

# ========== Main list gather ==========
def visit_and_collect(driver, url):
    links = set()
    driver.get(url)
    wait_ready(driver); time.sleep(1.0)

    for label in TAB_LABELS:
        _click_tab(driver, label)
        _load_everything_in_view(driver)
        links |= _collect_candidates_via_js(driver)

    # กันพลาด: โหลดทั้งหน้ารอบใหญ่ + เก็บซ้ำ
    _load_everything_in_view(driver)
    links |= _collect_candidates_via_js(driver)

    # fallbacks
    html = driver.page_source
    links |= _fallback_extract_from_html(html)
    links |= _extract_from_nuxt(driver)

    # กรองเฉพาะโปรไฟล์แบบ root ที่มีจุด
    links = {u for u in links if _is_profile_abs_url(u)}
    return links

def get_profile_links(driver):
    all_links = set()
    for path in ("/people", "/en/people", "/th/people"):
        try:
            all_links |= visit_and_collect(driver, urljoin(BASE, path))
        except Exception:
            pass

    # --debug: เขียนไฟล์ลิงก์
    if "--debug" in sys.argv:
        try:
            with open(os.path.join(os.path.dirname(__file__), "debug_links.txt"), "w", encoding="utf-8") as f:
                for u in sorted(all_links):
                    f.write(u + "\n")
        except Exception:
            pass

    return sorted(all_links)

# ========== Profile parsing ==========
def text_or(meta): return meta.get("content","").strip() if meta else ""

def extract_names_from_title(title_text):
    name_th, name_en = "", ""
    if title_text:
        m = re.search(r"([A-Za-z].+)", title_text)
        if m:
            name_th = title_text[:m.start()].strip(" ()-–—\u200b")
            name_en = m.group(1).strip(" ()-–—\u200b")
        else:
            name_th = title_text.strip()
    return name_th, name_en

EDU_TAB_TEXTS = ["การศึกษา","Education","ประวัติการศึกษา"]
EDU_KEYWORDS = ("การศึกษา","วุฒิ","วุฒิการศึกษา","ประวัติการศึกษา","ปริญญา","ระดับ",
                "มหาวิทยาลัย","University","Bachelor","Master","Ph.D","PhD","Degree")

def _has_edu_keyword(t): 
    if not t: return False
    low = t.lower()
    return any(k.lower() in low for k in EDU_KEYWORDS)

def _clean_text(s): return re.sub(r"\s+"," ", s or "").strip()

def parse_education(soup) -> str:
    if not soup: return ""
    tb_list = soup.select("table")
    for tb in tb_list:
        rows = []
        trs = tb.select("tbody tr") or tb.select("tr")
        for tr in trs:
            if tr.find("th"): continue
            for td in tr.find_all("td"):
                for br in td.find_all("br"): br.replace_with("\n")
            cols = [_clean_text(td.get_text(" ", strip=True)) for td in tr.find_all("td")]
            line = " | ".join([c for c in cols if c])
            if line: rows.append(line)
        if rows: return "\n".join(rows)
    items = [_clean_text(li.get_text(" ", strip=True)) for li in soup.select("ul li, ol li")]
    items = [i for i in items if i]
    if items: return "\n".join(items)
    blocks = []
    for tag in soup.select("p, div, span"):
        for br in tag.find_all("br"): br.replace_with("\n")
        t = tag.get_text("\n", strip=True)
        for ln in (s.strip() for s in t.split("\n") if s.strip()):
            if _has_edu_keyword(ln) or re.search(r"(Bachelor|Master|Ph\.?D|มหาวิทยาลัย|ปริญญา|วุฒิ)", ln, re.I):
                blocks.append(_clean_text(ln))
    if blocks:
        seen, out = set(), []
        for b in blocks:
            if b not in seen: seen.add(b); out.append(b)
        return "\n".join(out)
    return _clean_text(soup.get_text(" ", strip=True))

def parse_info_and_position(soup):
    texts = []
    for cand in soup.select("ul,ol,p,table,tr,td,div,span"):
        t = cand.get_text(" ", strip=True)
        if t and len(t) > 3: texts.append(t)
    info_text = "\n".join(texts).strip()
    position = ""
    for line in info_text.splitlines():
        if "ตำแหน่ง" in line:
            m = re.search(r"ตำแหน่ง[:：]?\s*(.+)", line)
            if m: position = m.group(1).strip(); break
    return info_text, position

def scrape_profile(driver, url):
    driver.get(url)
    wait_ready(driver); time.sleep(0.35)
    soup = BeautifulSoup(driver.page_source, "html.parser")

    title = text_or(soup.select_one('meta[property="og:title"]'))
    og_image = text_or(soup.select_one('meta[property="og:image"]'))
    name_th, name_en = extract_names_from_title(title)
    if not (name_th or name_en):
        h = soup.find(["h1","h2","h3"])
        if h: name_th, name_en = extract_names_from_title(h.get_text(" ", strip=True))

    info_text, position = parse_info_and_position(soup)

    # ลองเปิดแท็บการศึกษา (ถ้ามี)
    education = ""
    for t in EDU_TAB_TEXTS:
        tabs = driver.find_elements(By.CSS_SELECTOR, "[role='tab'], .v-tab, button, a")
        trg = None
        for x in tabs:
            txt = (x.text or "").strip()
            if t in txt: trg = x; break
        if trg:
            try:
                driver.execute_script("arguments[0].scrollIntoView({block:'center'});", trg)
                time.sleep(0.1)
                try: trg.click()
                except Exception: driver.execute_script("arguments[0].click();", trg)
                time.sleep(0.4)
                education = parse_education(BeautifulSoup(driver.page_source, "html.parser")).strip()
                if education: break
            except Exception:
                pass

    # email
    email = ""
    a = soup.select_one('a[href^="mailto:"]')
    if a: email = (a.get_text(strip=True) or a.get("href","").replace("mailto:","").strip())
    if not email:
        m = re.search(r"[A-Za-z0-9._%+-]+@kku\.ac\.th", soup.get_text(" ", strip=True))
        email = m.group(0) if m else ""

    # รูป
    photo_url = og_image
    if not photo_url:
        for img in soup.find_all("img"):
            src = (img.get("src") or "").strip()
            if not src: continue
            low = src.lower()
            if any(x in low for x in ["icon","logo","favicon","sprite","_nuxt/img/en"]): continue
            photo_url = src if src.startswith("http") else urljoin(BASE, src)
            break

    return {
        "name_th": name_th,
        "name_en": name_en,
        "position": position,
        "email": email,
        "photo_url": photo_url,
        "info": info_text,
        "education": education,
        "profile_url": _norm_abs(url),
    }

# ========== Entry ==========
def main():
    debug = "--debug" in sys.argv
    eprint("RUNNING FILE:", __file__)

    driver = make_driver()
    try:
        # เก็บจาก /people, /en/people, /th/people
        links = set()
        for p in ("/people", "/en/people", "/th/people"):
            try:
                links |= visit_and_collect(driver, urljoin(BASE, p))
            except Exception:
                pass

        links = sorted(links)
        eprint(f"Found {len(links)} profile links")

        if debug:
            try:
                with open(os.path.join(os.path.dirname(__file__), "debug_links.txt"), "w", encoding="utf-8") as f:
                    for u in links: f.write(u + "\n")
            except Exception:
                pass

        people = []
        for u in links:
            try:
                people.append(scrape_profile(driver, u))
            except Exception:
                # ถ้าคนใดคนหนึ่ง error ไม่ให้ล้มทั้งล็อต
                pass

        out_json = json.dumps(people, ensure_ascii=False, indent=2)
        out_path = os.path.join(os.path.dirname(__file__), "kku_people.json")
        with open(out_path, "w", encoding="utf-8-sig", newline="\n") as f:
            f.write(out_json)

        try: print(out_json)
        except UnicodeEncodeError: pass

        eprint(f"Saved JSON to {out_path}")
    finally:
        driver.quit()

if __name__ == "__main__":
    main()
