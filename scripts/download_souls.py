import os
import re
import asyncio
import aiohttp

# Target directory in the backend workspace presets
SAVE_DIR = os.path.abspath(
    os.path.join(
        os.path.dirname(__file__),
        "../backend/internal/workspace/presets/souls"
    )
)
os.makedirs(SAVE_DIR, exist_ok=True)

async def download_soul(session, sem, url, handle, slug):
    async with sem:
        author_dir = os.path.join(SAVE_DIR, handle)
        os.makedirs(author_dir, exist_ok=True)
        file_path = os.path.join(author_dir, f"{slug}.md")
        
        # Skip if already exists and is non-empty
        if os.path.exists(file_path) and os.path.getsize(file_path) > 0:
            return
            
        retries = 5
        backoff = 3
        for attempt in range(retries):
            try:
                timeout = aiohttp.ClientTimeout(total=15)
                async with session.get(url, timeout=timeout) as response:
                    if response.status == 200:
                        content = await response.text()
                        with open(file_path, "w", encoding="utf-8") as f:
                            f.write(content)
                        print(f"成功下载: {handle}/{slug}")
                        return
                    elif response.status == 429:
                        retry_after = response.headers.get("Retry-After")
                        wait_time = int(retry_after) if retry_after and retry_after.isdigit() else backoff
                        print(f"遇到限流 (429): {handle}/{slug}，等待 {wait_time} 秒后重试 (第 {attempt + 1}/{retries} 次)...")
                        await asyncio.sleep(wait_time)
                        backoff *= 2
                    else:
                        print(f"下载失败 ({response.status}): {handle}/{slug}")
                        return
            except Exception as e:
                if attempt == retries - 1:
                    print(f"请求异常 (已达最大重试): {handle}/{slug} | {e}")
                else:
                    await asyncio.sleep(backoff)
                    backoff *= 2
        
        # 强制请求完后的基本休眠，以控制总体请求频率
        await asyncio.sleep(0.5)

async def main():
    print(f"目标保存目录: {SAVE_DIR}")
    print("正在获取 souls.directory 的全量索引...")
    
    async with aiohttp.ClientSession() as session:
        try:
            async with session.get("https://souls.directory/llms.txt", timeout=20) as response:
                if response.status != 200:
                    print("获取索引失败！")
                    return
                index_text = await response.text()
        except Exception as e:
            print(f"获取索引异常: {e}")
            return
            
    # 正则提取所有 Souls 的 API 链接
    api_pattern = r"GET (https://souls.directory/api/souls/([^/]+)/([^/]+)\.md)"
    matches = re.findall(api_pattern, index_text)
    
    # 过滤掉说明文字中的占位符 {handle} 和 {slug}
    valid_matches = []
    for url, handle, slug in matches:
        if "{" in handle or "}" in handle or "{" in slug or "}" in slug:
            continue
        valid_matches.append((url, handle, slug))
        
    print(f"解析完成，共发现 {len(valid_matches)} 个有效的 Soul 模板。")
    
    # 降低并发到 2 并发以避免触发过于频繁的限流
    sem = asyncio.Semaphore(2)
    
    async with aiohttp.ClientSession() as session:
        tasks = []
        for url, handle, slug in valid_matches:
            tasks.append(download_soul(session, sem, url, handle, slug))
        
        await asyncio.gather(*tasks)

if __name__ == "__main__":
    asyncio.run(main())
