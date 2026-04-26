# GPT Image 多图生成 / 图生图 对接文档

本文档专门说明**多图生成**和**图生图（edits）**的请求 / 响应格式，便于下游正确解析。

---

## 1. 关键结论（先看这个）

| 问题                    | 结论 |
|------------------------|---|
| `n` 字段有用吗？           | **没用**。底层 Codex 的 `image_generation` tool 不接受 `n / count / num_images` 任何字段，sub2api 也不会透传它。 |
| 那怎么生成多张？             | **在 `prompt` 里面用自然语言要求**，例如 `请返回4张同个人不同姿势的独立图片，不要拼接成一张图`。 |
| 下游需要指定张数吗？           | **不需要**。下游只需要解析返回 JSON 里 `data` 数组的长度即可，长度就是真实图片数。|
| 不指定数量会怎样？            | 默认返回 1 张。 |
| 上游是否保证一定按 prompt 数量返回？ | 不保证 100%。模型可能给出少于 / 多于请求的张数（边缘情况），所以下游必须以 `data` 数组的实际长度为准。|
| 图生图（edits）和文生图（generations）是否一致？ | **完全一致**。两个端点的请求字段不同，但响应结构相同。|

---

## 2. 请求

### 端点

| Method | Path                     | 用途        |
|--------|--------------------------|-----------|
| POST   | `/v1/images/generations` | 文生图       |
| POST   | `/v1/images/edits`       | 图生图（含 mask）|

### 鉴权

```
Authorization: Bearer sk-xxxxxxxxxxxxxxxx
```

### 文生图请求（JSON）

```bash
curl -X POST {BASE}/v1/images/generations \
  -H "Authorization: Bearer sk-xxxxxxxxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "y2k 风格 韩国少女街拍，请返回4张同人不同姿势的独立图片，不要拼接",
    "size": "1024x1024"
  }'
```

> **不要**传 `"n": 4`。无效。直接在 prompt 里说"4 张"。

### 图生图请求（multipart）

```bash
curl -X POST {BASE}/v1/images/edits \
  -H "Authorization: Bearer sk-xxxxxxxxxxxxxxxx" \
  -F "model=gpt-image-2" \
  -F "size=1024x1024" \
  -F "prompt=y2k 风格 韩国少女街拍，请返回4张同人不同姿势的独立图片，不要拼接" \
  -F "image=@/path/to/source.jpg"
```

可选字段（与官方一致）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `model` | string | `gpt-image-2`（默认）/ `gpt-image-1.5` / `gpt-image-1` |
| `prompt` | string | **必填**。如要多图，在文本里要求张数 |
| `size` | string | `1024x1024` / `1536x1024` / `1024x1536` 等 |
| `response_format` | string | `b64_json`（默认）或 `url` |
| `image` | file | 仅 edits 端点必填，可上传多张作为参考 |
| `mask` | file | 仅 edits 端点可选，透明区域为可编辑区域 |
| `quality`, `background`, `style`, `output_format`, `output_compression`, `moderation` | — | 透传给上游的高级参数 |
| `n` | int | **会被忽略**（保留以兼容 OpenAI SDK，但不影响实际生成数量） |

---

## 3. 响应格式

### 非流式（默认）

返回 `Content-Type: application/json`，结构与 OpenAI 官方完全一致：

```json
{
  "created": 1777088842,
  "data": [
    { "b64_json": "iVBORw0KGgoAAAANSUhEUgAA...（约 2MB base64）" },
    { "b64_json": "iVBORw0KGgoAAAANSUhEUgAA..." },
    { "b64_json": "iVBORw0KGgoAAAANSUhEUgAA..." },
    { "b64_json": "iVBORw0KGgoAAAANSUhEUgAA..." }
  ],
  "size": "1024x1024",
  "output_format": "png",
  "model": "gpt-image-2"
}
```

**下游解析规则：**
1. 直接读 `data` 数组
2. `data.length` 就是真实图片数（可能少于/多于 prompt 中要求的数量）
3. 每个元素是一张独立完整的图片
4. 如果发请求时 `response_format=url`，则每个元素是 `{ "url": "data:image/png;base64,..." }` 而不是 `{ "b64_json": "..." }`
5. `revised_prompt` 字段在每个元素里可能出现（上游对你的 prompt 做了改写时）

#### Python 解析示例

```python
import base64, json, requests

resp = requests.post(
    f"{BASE}/v1/images/edits",
    headers={"Authorization": f"Bearer {KEY}"},
    files={"image": open("source.jpg", "rb")},
    data={
        "model": "gpt-image-2",
        "size": "1024x1024",
        "prompt": "y2k 风格 韩国少女街拍，请返回4张同个人不同姿势的独立图片，不要拼接",
    },
    timeout=600,
)
result = resp.json()
print(f"上游实际返回 {len(result['data'])} 张图")
for i, item in enumerate(result["data"]):
    img_bytes = base64.b64decode(item["b64_json"])
    with open(f"out_{i}.png", "wb") as f:
        f.write(img_bytes)
```

#### Node 解析示例

```javascript
const res = await fetch(`${BASE}/v1/images/edits`, {
  method: "POST",
  headers: { Authorization: `Bearer ${KEY}` },
  body: form, // FormData with image + model + prompt
});
const json = await res.json();
console.log(`上游实际返回 ${json.data.length} 张图`);
json.data.forEach((item, i) => {
  // item.b64_json 或 item.url（取决于 response_format）
  fs.writeFileSync(`out_${i}.png`, Buffer.from(item.b64_json, "base64"));
});
```

### 流式（`stream: true`）

`Content-Type: text/event-stream`，事件序列（**每张图都会产生独立的 partial_image 和 completed 事件**）：

```
event: image_edit.partial_image
data: {"type":"image_edit.partial_image","created_at":1777088842,"partial_image_index":0,"b64_json":"...","output_format":"png","size":"1024x1024"}

event: image_edit.partial_image
data: {"type":"image_edit.partial_image","created_at":1777088842,"partial_image_index":0,"b64_json":"...","output_format":"png","size":"1024x1024"}

...（重复 N 次，每张图可能有多个 partial 事件）

event: image_edit.completed
data: {"type":"image_edit.completed","created_at":1777088842,"b64_json":"...完整图1的base64...","output_format":"png","size":"1024x1024","usage":{...}}

event: image_edit.completed
data: {"type":"image_edit.completed","created_at":1777088842,"b64_json":"...完整图2..."}

event: image_edit.completed
data: {"type":"image_edit.completed","created_at":1777088842,"b64_json":"...完整图3..."}

event: image_edit.completed
data: {"type":"image_edit.completed","created_at":1777088842,"b64_json":"...完整图4..."}
```

> 文生图模式 event 名是 `image_generation.partial_image` / `image_generation.completed`，结构相同。

**下游流式解析：**
- 计数 `*.completed` 事件的数量 = 真实图片数
- 每个 `*.completed` 的 `b64_json` 字段是一张完整图
- `*.partial_image` 是渐进式预览，可丢弃或用于 UI 渐进展示
- 不需要等所有 partial 才能落盘 completed —— completed 事件本身就含完整图

---

## 4. 错误响应

| HTTP | 场景 |
|---|---|
| 400 | 参数错误（如 prompt 空、edits 缺 image 文件） |
| 401 | API Key 无效 |
| 404 | API Key 绑定的 group 不是 OpenAI 平台 |
| 429 | 上游账号配额或 rate limit |
| 502 | 上游账号不支持图像生成（如绑定了 ChatGPT Free 账号），或上游临时故障 |

错误体：

```json
{
  "error": {
    "type": "upstream_error",
    "message": "Upstream request failed",
    "code": null
  }
}
```

---

## 5. 超时与大图建议

- 网关单次请求超时 **600 秒**（10 分钟）
- 多图生成耗时 ≈ 单图耗时 × N（不是并行）；4 张大约 5-8 分钟，可能接近超时上限
- **建议下游 client 端 timeout ≥ 10 分钟**
- 如果客户端常超时，把生成数量控制在 ≤ 4 张为佳

---

## 6. 关键 FAQ

**Q: 我前端用 OpenAI SDK 自动加了 `n: 4`，会出问题吗？**
不会出错，但 `n` 完全被忽略。**你必须在 prompt 里告诉模型生成几张**。

**Q: 上游有时候只返回 1 张图，怎么办？**
模型偶发不遵循指令。建议：
1. Prompt 里把数量写得更清楚明确（例如开头就说"生成4张图片，每张不同姿势"）
2. 强调"独立图片"、"不要拼接成一张"，避免模型把 4 个画面合并成一张大图
3. 下游 UI 容错：以 `data.length` 为准展示，不假设固定数量

**Q: 下游能要求一次返回 8 张以上吗？**
理论上可以，但耗时和失败率会显著上升。建议 ≤ 6 张。

**Q: 不同账号是不是有不同支持？**
是。**ChatGPT Free 账号不支持** image_generation tool，会返回 400 "Tool choice 'image_generation' not found"。需要 Plus / Pro 账号。后台调度器目前没做 plan 过滤，free 账号会偶发被选中导致单次失败（一般会自动 failover 到下个账号）。

**Q: 图生图传多张参考图可以吗？**
可以，多次 `-F "image=@a.jpg" -F "image=@b.jpg"` 即可，全部作为参考输入给模型。
