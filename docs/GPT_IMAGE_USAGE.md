# GPT Image API 对接文档（最终版）

> 基于实测 + 代码审计，覆盖 sub2api 当前真实支持的字段、值、分辨率、计费规则。

---

## 一、端点

| Method | Path                     | 用途        | Content-Type                |
|--------|--------------------------|-----------|-----------------------------|
| POST   | `/v1/images/generations` | 文生图       | `application/json`          |
| POST   | `/v1/images/edits`       | 图生图（含 mask）| `multipart/form-data`       |

> 不带 `/v1` 前缀的 `/images/generations` 与 `/images/edits` 等价。

## 二、鉴权

```
Authorization: Bearer sk-xxxxxxxxxxxxxxxx
```

**关键前置条件**：API Key 绑定的 group 必须满足
- `platform = openai`
- group 内至少含 1 个 `plan_type ∈ {pro, plus}` 的 OAuth 账号

否则会得到：
- `404 Not Found` —— group platform 不对
- `502 Upstream request failed` —— 命中 free 账号（gpt-image tool 不可用）

---

## 三、支持的分辨率

### 计费档位（sub2api 内部）
| 档位 | 长边 | 总像素 |
|---|---|---|
| **1K** | ≤ 1024 | ≤ ~1.05 MP |
| **2K** | < 2048 | 1.05 ~ 2.5 MP |
| **4K** | ≥ 2048 或 ≥ 2.5 MP | — |

> 计费用 `Group.ImagePrice1K / 2K / 4K`（管理台配置），fallback 时按 LiteLLM 基础价 × {1, 1.5, 2} 倍率。

### 常用分辨率清单（实测可用）

#### 1K 档（速度最快、最便宜）
| `size` | 比例 | 像素 | 用途 |
|---|---|---|---|
| `1024x1024` | 1:1 | 1024×1024 | 头像、社交方图 |
| `1024x768` | 4:3 | 0.79 MP | 横屏经典 |
| `768x1024` | 3:4 | 0.79 MP | 竖屏经典 |

#### 2K 档（高清，平衡）
| `size` | 比例 | 像素 | 用途 |
|---|---|---|---|
| `1536x1024` | 3:2 | 1.57 MP | 横版海报 |
| `1024x1536` | 2:3 | 1.57 MP | 竖版海报 |
| `1792x1024` | ≈16:9 | 1.84 MP | 视频封面横屏 |
| `1024x1792` | ≈9:16 | 1.84 MP | 手机竖屏 |
| `1536x1536` | 1:1 大 | 2.36 MP | 中清方图 |

#### 4K 档（超清）
| `size` | 比例 | 像素 | 用途 |
|---|---|---|---|
| `1792x1792` | 1:1 | 3.21 MP | 大方图 |
| `2048x2048` | 1:1 | 4.19 MP | 高清方图 |
| `2048x1024` | 2:1 | 2.10 MP | 超宽横幅 |
| `1024x2048` | 1:2 | 2.10 MP | 超长竖图 |
| **`3840x2160`** | **16:9** | **8.29 MP** | **真 4K 视频帧、桌面壁纸** |
| `2160x3840` | 9:16 | 8.29 MP | 4K 竖屏 |

#### 自动 / 默认
- `size=auto` 或不传 → 模型自选（通常 1024×1024）

#### ❌ 不支持
- 任意一边 < 768（实测 `256x256` / `512x512` 上游 400）

---

## 四、请求字段全表

| 字段 | 类型 | 可选值 | 默认 | 说明 |
|---|---|---|---|---|
| `model` | string | `gpt-image-2` / `gpt-image-1.5` / `gpt-image-1` | `gpt-image-2` | 生图模型 |
| `prompt` | string | 任意文本 | **必填** | 提示词；要多张图直接在 prompt 里写"返回 N 张"（见下方 §六）|
| `size` | string | 见 §三 | `auto` | 分辨率（透传上游）|
| `quality` | string | `low` / `medium` / `high` / `auto` | 不传（≈auto）| 渲染质量。**不改像素，改细节**。其它值会被 sub2api 静默丢弃 |
| `output_format` | string | `png` / `jpeg`（`jpg` 视为 `jpeg`）| `png` | **`webp` 不支持**：sub2api 会静默改为 png |
| `output_compression` | int | 0~100（超界 clamp）| — | 仅 `jpeg` 有效，越大越压缩（文件越小） |
| `background` | string | `opaque` / `auto` | 不传 | **`transparent` 不支持**：sub2api 会静默改为 `auto` |
| `moderation` | string | `auto` / `low` | `auto` | 内容审核严格度，其它值丢弃 |
| `style` | — | — | — | **完全不支持**：上游 image_generation tool 不接受。sub2api 会静默丢弃 |
| `response_format` | string | `b64_json` / `url` | `b64_json` | 响应中图片字段类型 |
| `image` | file | 任意 | edits **必填** | 仅 edits 端点；可多次重复 `-F image=@x` 传多图作为参考 |
| `mask` | file | 任意（同尺寸 PNG）| — | 仅 edits 端点；透明像素区域为可编辑区域 |
| `n` | int | 1+ | 1 | **被忽略**！要多图请见 §六 |
| `stream` | bool | true / false | false | SSE 流式返回（每张图独立的 partial + completed 事件）|

### 字段适配规则（来自客户端 → 上游）

sub2api 会对所有字段做"清洗"，规则简单：
- 合法值 → 透传给上游
- 非法值（如 `style=vivid`、`output_format=bmp`、`background=transparent`）→ 静默丢弃或回退到安全值
- 空值 → 等同于不传（上游用默认）
- 客户端不传 → 上游不收到该字段（不强加默认）

> **客户端不需要预先验证字段值**。sub2api 网关会保证上游不收到无效值，避免 502。

---

## 五、响应格式

### 5.1 非流式（默认 `stream=false`）

`Content-Type: application/json`

```json
{
  "created": 1777088842,
  "data": [
    {
      "b64_json": "iVBORw0KGgoAAAANSUhEUgAA...",
      "revised_prompt": "可选，上游对 prompt 的改写"
    }
  ],
  "size": "3840x2160",
  "output_format": "png",
  "model": "gpt-image-2"
}
```

**下游解析规则**：
- `data` 是数组，长度 = 实际图片数
- 每个元素 `{b64_json}`（默认）或 `{url: "data:image/...;base64,..."}`（`response_format=url`）
- `b64_json` 是完整 base64 PNG/JPEG，直接 `base64.decode()` 落盘即可

### 5.2 流式（`stream=true`）

`Content-Type: text/event-stream`

事件序列（图生图 prefix 是 `image_edit.*`，文生图是 `image_generation.*`）：

```
event: image_edit.partial_image
data: {"type":"image_edit.partial_image","created_at":1777,...,"partial_image_index":0,"b64_json":"..."}

event: image_edit.partial_image
data: {...同上, partial_image_index 递增...}

event: image_edit.completed
data: {"type":"image_edit.completed","created_at":1777,...,"b64_json":"...完整图1...","output_format":"png","size":"...","usage":{...}}

（多图时重复 N 次 completed 事件，每个对应一张完整图）
```

**下游流式解析**：
- 计 `*.completed` 事件数 = 真实图片数
- 每个 `*.completed` 的 `b64_json` 字段是一张完整图
- `*.partial_image` 是渐进预览，可用于 UI 实时展示，**完整图最终落在 completed 事件**

---

## 六、多图生成

**`n` 字段被忽略**。要让上游返回多张图，请在 prompt 里用自然语言要求：

```bash
-F "prompt=请返回4张同人不同姿势的独立图片，不要拼接成一张图"
```

**特征**：
- **同一次调用**，上游模型在一个 reasoning 链里串行生成 N 张
- 风格一致性高（同一上下文）
- 串行耗时 ≈ N × 单图耗时
- 不保证严格按 prompt 数量返回（模型偶发返回少 / 多 1 张）—— **下游必须以 `data.length` 为准**

---

## 七、计费

### 单价决定（自上而下）

```
1) 渠道定价（admin 在管理台为模型在 group 下设的"渠道定价"）
   ↓ 不存在则
2) 分组定价（Group.ImagePrice1K / 2K / 4K）
   ↓ 不存在则
3) LiteLLM 默认（PricingService.GetModelPricing(model).OutputCostPerImage）
   ↓ 不存在则
4) 硬编码兜底（$0.134，源自 gemini-3-pro-image-preview）

档位倍率（仅在第 3、4 层 fallback 时生效）：
  1K = base × 1
  2K = base × 1.5
  4K = base × 2
```

### 总成本公式

```
unitPrice = （档位查到的单价）
totalCost = unitPrice × imageCount    (imageCount = data.length)
actualCost = totalCost × rateMultiplier
```

`rateMultiplier` 来自账号或 group 的 `rate_multiplier` 字段，默认 1.0。

### 落库字段（`usage_logs`）

```sql
SELECT created_at, model, image_count, image_size,
       total_cost, actual_cost, billing_mode
FROM usage_logs
WHERE billing_mode = 'image'
ORDER BY created_at DESC
LIMIT 10;
```

`image_size` 取值：`"1K"` / `"2K"` / `"4K"`（**4K 计费 bug 已修**，原代码所有非官方尺寸都被错归为 2K）。

---

## 八、超时与性能（实测）

| 场景 | 平均耗时 | 客户端 timeout 建议 |
|---|---|---|
| 1K 单图 | 30-60 s | ≥ 120 s |
| 2K 单图 | 60-120 s | ≥ 180 s |
| 4K 单图 | 50-90 s | ≥ 240 s |
| 多图 4 张 1K | ~5 分钟 | ≥ 600 s |
| 多图 4 张 4K | 8-10 分钟 | **≥ 600 s（已是网关上限）** |

> **网关硬上限 600 秒**。客户端 timeout 比这小没用；比这大也没用（网关会先 504）。

---

## 九、错误码

| HTTP | 场景 | 客户端动作 |
|---|---|---|
| 400 | 参数无效（`size` 太小、prompt 空、edits 缺 image）| 修正参数 |
| 401 | API Key 无效/过期 | 换 key |
| 404 | group 平台不是 OpenAI | 换 key（绑到 OpenAI group）|
| 429 | 上游限流 | sub2api 已自动重试其它账号；客户端可稍后重试 |
| 502 | 上游错误 / failover 用尽 | 一般偶发，可重试一次 |
| 504 | 单请求 > 600 秒 | 减少 prompt 复杂度或图片张数 |

错误体：
```json
{
  "error": {
    "type": "upstream_error",
    "message": "...",
    "code": null
  }
}
```

---

## 十、Python 完整示例

```python
import base64
import requests

BASE = "https://你的域名"
KEY  = "sk-xxxxxxxxxxxxxxxx"

# === 文生图 4K ===
resp = requests.post(
    f"{BASE}/v1/images/generations",
    headers={"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"},
    json={
        "model": "gpt-image-2",
        "prompt": "a futuristic city at sunset, ultra detailed",
        "size": "3840x2160",
        "quality": "high",
        "output_format": "png",
    },
    timeout=600,
)
result = resp.json()
print(f"返回 {len(result['data'])} 张, size={result.get('size')}")
for i, item in enumerate(result["data"]):
    with open(f"out_{i}.png", "wb") as f:
        f.write(base64.b64decode(item["b64_json"]))

# === 图生图 + 多图（prompt 里要求 4 张）===
resp = requests.post(
    f"{BASE}/v1/images/edits",
    headers={"Authorization": f"Bearer {KEY}"},
    files={"image": open("source.jpg", "rb")},
    data={
        "model": "gpt-image-2",
        "size": "1024x1024",
        "quality": "medium",
        "output_format": "jpeg",
        "output_compression": "85",
        "prompt": "y2k 风格少女街拍，请返回 4 张同人不同姿势的独立图片，不要拼接",
    },
    timeout=600,
)
result = resp.json()
for i, item in enumerate(result["data"]):
    with open(f"edit_{i}.jpeg", "wb") as f:
        f.write(base64.b64decode(item["b64_json"]))
```

## 十一、Node 完整示例

```javascript
import fs from "node:fs";

const BASE = "https://你的域名";
const KEY  = "sk-xxxxxxxxxxxxxxxx";

// === 图生图 4K ===
const form = new FormData();
form.append("model", "gpt-image-2");
form.append("size", "3840x2160");
form.append("quality", "high");
form.append("prompt", "convert this photo into a vaporwave 4k wallpaper");
form.append("image", new Blob([fs.readFileSync("source.jpg")]), "source.jpg");

const res = await fetch(`${BASE}/v1/images/edits`, {
  method: "POST",
  headers: { Authorization: `Bearer ${KEY}` },
  body: form,
  signal: AbortSignal.timeout(600_000),
});
const json = await res.json();

console.log(`返回 ${json.data.length} 张`);
json.data.forEach((item, i) => {
  fs.writeFileSync(`out_${i}.png`, Buffer.from(item.b64_json, "base64"));
});
```

---

## 十二、关键提醒（精简版）

1. **`n` 字段无效** — 多图请求请在 `prompt` 里用自然语言写"返回 N 张"
2. **`style` / `webp` / `transparent` 不支持** — sub2api 会静默修正，**不会 502**
3. **真 4K 可用** — `size=3840x2160`，且按 4K 档真实计费
4. **多图风格一致性高，但耗时 = N × 单图**
5. **客户端 timeout ≥ 600 秒**，超过网关也会 504
6. **响应是 b64 内嵌 JSON**，4K 响应体可达 8 MB+，下游 buffer 准备充足
7. **Free 账号偶发被调度** → 单次 502，建议运维侧把 `plan_type=free` 的账号 `schedulable=false`
