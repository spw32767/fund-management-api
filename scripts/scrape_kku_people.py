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
            conditions = " or ".join(
                [
                    f"contains(translate(normalize-space(.), 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz'), '{t.lower()}')"
                    for t in texts
                ]
            )
            xp = f".//*[self::a or self::button or @role='button'][{conditions}]"
            els = scope.find_elements(By.XPATH, xp)
        except Exception:
            els = []
        for el in els:
            try:
                driver.execute_script("arguments[0].scrollIntoView({block:'center'});", el)
                time.sleep(0.1)
                if el.is_displayed() and el.is_enabled():
                    try:
                        el.click()
                    except Exception:
                        driver.execute_script("arguments[0].click();", el)
                    time.sleep(0.6)
                    clicked = True
                    break
            except Exception:
                pass
        if not clicked:
            break

def _load_everything_in_view(driver):
    _click_texty_buttons(
        driver, driver,
        ["โหลดเพิ่ม","โหลดเพิ่มเติม","ดูเพิ่มเติม","เพิ่มเติม",
         "Load more","More","Show more","Next","ถัดไป"],
         max_round=80
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
ALLOWED_SEGMENT_RE = re.compile(r"^[A-Za-z0-9_.\-ก-๙]+$")  # ไทย/ขีดกลาง/ขีดล่าง/จุด

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

def _is_profile_abs_url(abs_url: str) -> bool:
    """
    ยอมรับเฉพาะโปรไฟล์แบบ root 1 segment ที่มีจุด:
    /firstname.lastname
    (ตัดทิ้งทุกอย่างที่ขึ้นต้นด้วย /en/ หรือ /th/)
    """
    if not abs_url or not abs_url.startswith(BASE): return False
    p = urlparse(abs_url); path = p.path or ""
    if not path or path == "/": return False
    if path.startswith("/en/") or path.startswith("/th/"): return False
    if STATIC_EXT.search(path): return False

    segs = [s for s in path.split("/") if s]  # ไม่ตัด prefix ภาษาแล้ว
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
    for m in re.finditer(r'href=["\'](/[^"\']]{1,200})["\']', html):
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

    # กันพลาด: โหลดทั้งหน้ารอบใหญ่ + เก็บซ้ำ + fallbacks
    _load_everything_in_view(driver)
    links |= _collect_candidates_via_js(driver)
    html = driver.page_source
    links |= _fallback_extract_from_html(html)
    links |= _extract_from_nuxt(driver)

    # กรองเฉพาะโปรไฟล์ root 1 segment ที่มีจุด (ตัด /en/, /th/ ออก)
    links = {u for u in links if _is_profile_abs_url(u)}
    return links

def get_profile_links(driver):
    # เข้าเฉพาะ /people เท่านั้น
    all_links = visit_and_collect(driver, urljoin(BASE, "/people"))

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

def parse_education(tab_soup):
    if not tab_soup:
        return ""
    return clean_text(tab_soup.get_text(separator="\n"))

EXCLUDE_LINES = [ "เกี่ยวกับเรา", "ติดต่อเรา", "เข้าสู่ระบบ", "ข่าวสาร",
"สิ่งอำนวยความสะดวก", "123 ถ.มิตรภาพ", "College of Computing", "ค้นหา", "A-", "A+", "|"]
def clean_text(text):
    lines = [l.strip() for l in text.splitlines() if l.strip()]
    return "\n".join(l for l in lines if not any(ex in l for ex in EXCLUDE_LINES))

def parse_info_and_position(tab_soup):
    if not tab_soup:
        return "", ""
    info_text = clean_text(tab_soup.get_text(separator="\n"))
    position = ""
    for tag in tab_soup.find_all(["p", "div", "span"]):
        if "อาจารย์" in tag.get_text():
            position = tag.get_text(strip=True)
            break
    return info_text, position

def scrape_profile(driver, url):
    driver.get(url)
    wait_ready(driver)
    # รอให้ข้อมูลที่โหลดด้วย JavaScript เข้ามาครบก่อน
    try:
        WebDriverWait(driver, 10).until(
            lambda d: any(k in d.page_source for k in ["วุฒิ","ปริญญา","Bachelor","Master","Ph.D","PhD","Degree"])
        )
    except Exception:
        pass
    soup = BeautifulSoup(driver.page_source, "html.parser")
    tabpanels = soup.select('div[role="tabpanel"]')
    info_tab = tabpanels[0] if len(tabpanels) > 0 else None
    edu_tab = tabpanels[1] if len(tabpanels) > 1 else None
    info_text, position = parse_info_and_position(info_tab)
    education = parse_education(edu_tab)

    title = text_or(soup.select_one('meta[property="og:title"]'))
    og_image = text_or(soup.select_one('meta[property="og:image"]'))
    name_th, name_en = extract_names_from_title(title)
    if not (name_th or name_en):
        h = soup.find(["h1","h2","h3"])
        if h: name_th, name_en = extract_names_from_title(h.get_text(" ", strip=True))

    # กรองข้อมูล info ไม่ให้มีข้อมูลการศึกษาปน
    if info_text:
        info_lines = []
        for line in info_text.splitlines():
            low_line = line.lower()
            if ("วุฒิ" in low_line or "วุฒิการศึกษา" in low_line or "ประวัติการศึกษา" in low_line or
                "ปริญญา" in low_line or "bachelor" in low_line or "master" in low_line or
                "ph.d" in low_line or "phd" in low_line or "degree" in low_line or "education" in low_line):
                continue
            info_lines.append(line)
        info_text = "\n".join(info_lines).strip()

    # การศึกษา: ดึงข้อมูลการศึกษาจากหน้าโดยตรง (ไม่ต้องคลิกแท็บ)
    education = parse_education(soup).strip()

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
        links = get_profile_links(driver)  # เฉพาะ /people
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
