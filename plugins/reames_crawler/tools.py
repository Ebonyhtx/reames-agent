"""Advanced web crawler tools."""
import requests
import re
import time
from urllib.parse import urljoin, urlparse

def _fetch(url):
    try:
        r = requests.get(url, timeout=15, headers={"User-Agent": "Mozilla/5.0"})
        return r.text, r.status_code
    except Exception as e:
        return str(e), 0

def _extract_links(html, base_url):
    links = set()
    for href in re.findall(r'href=["\'](https?://[^"\']+)["\']', html):
        links.add(href)
    for href in re.findall(r'href=["\'](/[^"\']+)["\']', html):
        links.add(urljoin(base_url, href))
    return links

def handle_crawl(args):
    """Recursively crawl a website up to a depth."""
    url = args.get("url", "")
    depth = args.get("depth", 1)
    if not url:
        return "Error: url is required"
    
    visited = set()
    to_visit = [(url, 0)]
    results = []
    
    while to_visit and len(results) < 50:
        current, d = to_visit.pop(0)
        if current in visited or d > depth:
            continue
        visited.add(current)
        html, status = _fetch(current)
        results.append(f"[{status}] {current} ({d} levels deep)")
        time.sleep(0.5)
        if d < depth:
            for link in _extract_links(html, current):
                if link not in visited:
                    to_visit.append((link, d + 1))
    
    return "Crawl results:\n" + "\n".join(results[:30])

CRAWLER_TOOLS = {
    "crawl_website": {"handler": handle_crawl, "description": "Recursively crawl a website"},
}
