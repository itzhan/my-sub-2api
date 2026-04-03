#!/bin/bash
#
# 临时不可调度账号处理脚本（并行版）
# - 查询所有临时不可调度的账号
# - 若原因包含 "deactivated" 关键词 → 直接删除账号
# - 其他原因 → 批量刷新令牌
# - 多号池并行处理，删除操作也并行（最多10并发）
#
# 用法: ./fix_temp_unschedulable.sh
# 配置: 编辑 fix_temp_unschedulable.conf 文件

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONF_FILE="${SCRIPT_DIR}/fix_temp_unschedulable.conf"

if [ ! -f "$CONF_FILE" ]; then
    echo "❌ 配置文件不存在: $CONF_FILE"
    echo "请先创建配置文件，格式参考 fix_temp_unschedulable.conf"
    exit 1
fi

# 每页查询数量
PAGE_SIZE=100
# 删除并发数
DELETE_CONCURRENCY=10

# 临时目录存放各号池结果
WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

process_server() {
    local name="$1"
    local base_url="$2"
    local api_key="$3"
    local log_file="$4"
    local stats_file="$5"

    # 去除末尾斜杠
    base_url="${base_url%/}"

    local deleted=0 refreshed=0 errors=0

    # 用于存放 curl 响应体的临时文件
    local body_file="${WORK_DIR}/body_$$_${RANDOM}.json"

    {
        echo ""
        echo "============================================"
        echo "🔄 处理号池: ${name}"
        echo "   地址: ${base_url}"
        echo "============================================"

        local page=1
        local total_pages=1
        local delete_ids=()
        local refresh_ids=()

        # 分页获取所有账号，找出临时不可调度的
        while [ "$page" -le "$total_pages" ]; do
            local http_code
            http_code=$(curl -s --connect-timeout 10 --max-time 30 \
                -o "$body_file" -w "%{http_code}" \
                -H "x-api-key: ${api_key}" \
                -H "Content-Type: application/json" \
                "${base_url}/api/v1/admin/accounts?page=${page}&page_size=${PAGE_SIZE}&status=active")

            if [ "$http_code" != "200" ]; then
                echo "   ❌ 请求失败 (HTTP ${http_code}), page=${page}"
                echo "   响应: $(head -c 200 "$body_file" 2>/dev/null)"
                errors=$((errors + 1))
                echo "${deleted} ${refreshed} ${errors}" > "$stats_file"
                return
            fi

            local body
            body=$(cat "$body_file")

            local code
            code=$(echo "$body" | jq -r '.code // -1')
            if [ "$code" != "0" ]; then
                echo "   ❌ API返回错误: $(echo "$body" | jq -r '.message // "unknown"')"
                errors=$((errors + 1))
                echo "${deleted} ${refreshed} ${errors}" > "$stats_file"
                return
            fi

            total_pages=$(echo "$body" | jq -r '.data.pages // 1')
            local total
            total=$(echo "$body" | jq -r '.data.total // 0')

            if [ "$page" -eq 1 ]; then
                echo "   📊 活跃账号总数: ${total}"
            fi

            # 用 jq 一次性提取所有临时不可调度账号，避免逐条调用 jq
            local unsched_items
            unsched_items=$(echo "$body" | jq -c '
                [.data.items[] |
                 select(.temp_unschedulable_until != null and .temp_unschedulable_until != "") |
                 {id, name: (.name // "unnamed"), reason: (.temp_unschedulable_reason // ""), until: .temp_unschedulable_until}
                ]')

            local unsched_count
            unsched_count=$(echo "$unsched_items" | jq 'length')

            local now_epoch
            now_epoch=$(date "+%s")

            for ((i=0; i<unsched_count; i++)); do
                local item
                item=$(echo "$unsched_items" | jq -c ".[$i]")

                local until_str
                until_str=$(echo "$item" | jq -r '.until')

                # 检查是否已过期
                local until_epoch
                until_epoch=$(date -j -f "%Y-%m-%dT%H:%M:%S" "$(echo "$until_str" | cut -d'.' -f1 | sed 's/Z$//')" "+%s" 2>/dev/null || date -d "$until_str" "+%s" 2>/dev/null || echo "0")

                if [ "$until_epoch" -le "$now_epoch" ] && [ "$until_epoch" -ne 0 ]; then
                    continue
                fi

                local acc_id acc_name reason
                acc_id=$(echo "$item" | jq -r '.id')
                acc_name=$(echo "$item" | jq -r '.name')
                reason=$(echo "$item" | jq -r '.reason')

                if echo "$reason" | grep -qi "deactivated"; then
                    echo "   🗑️  [删除] ID=${acc_id} 名称=${acc_name}"
                    echo "       原因: ${reason:0:120}"
                    delete_ids+=("$acc_id")
                else
                    echo "   🔄 [刷新] ID=${acc_id} 名称=${acc_name}"
                    echo "       原因: ${reason:0:120}"
                    refresh_ids+=("$acc_id")
                fi
            done

            page=$((page + 1))
        done

        echo ""
        echo "   📋 汇总: 需删除=${#delete_ids[@]}, 需刷新=${#refresh_ids[@]}"

        # 并行删除
        if [ ${#delete_ids[@]} -gt 0 ]; then
            echo ""
            echo "   🗑️  开始并行删除 ${#delete_ids[@]} 个已停用账号（并发=${DELETE_CONCURRENCY}）..."

            local del_results_dir="${WORK_DIR}/del_${name//[^a-zA-Z0-9]/_}"
            mkdir -p "$del_results_dir"

            local running=0
            for del_id in "${delete_ids[@]}"; do
                (
                    local del_code
                    del_code=$(curl -s --connect-timeout 10 --max-time 15 \
                        -o /dev/null -w "%{http_code}" \
                        -X DELETE \
                        -H "x-api-key: ${api_key}" \
                        "${base_url}/api/v1/admin/accounts/${del_id}")
                    if [ "$del_code" = "200" ]; then
                        echo "ok" > "${del_results_dir}/${del_id}"
                    else
                        echo "fail:${del_code}" > "${del_results_dir}/${del_id}"
                    fi
                ) &
                running=$((running + 1))
                if [ "$running" -ge "$DELETE_CONCURRENCY" ]; then
                    wait
                    running=0
                fi
            done
            wait

            # 汇总删除结果
            for del_id in "${delete_ids[@]}"; do
                local result_file="${del_results_dir}/${del_id}"
                if [ -f "$result_file" ] && [ "$(cat "$result_file")" = "ok" ]; then
                    echo "      ✅ 已删除 ID=${del_id}"
                    deleted=$((deleted + 1))
                else
                    local fail_info=""
                    [ -f "$result_file" ] && fail_info="$(cat "$result_file")"
                    echo "      ❌ 删除失败 ID=${del_id} (${fail_info})"
                    errors=$((errors + 1))
                fi
            done
        fi

        # 批量刷新（这个接口本身就是批量的，一次调用即可）
        if [ ${#refresh_ids[@]} -gt 0 ]; then
            echo ""
            echo "   🔄 开始刷新 ${#refresh_ids[@]} 个账号的令牌..."

            local ids_json
            ids_json=$(printf '%s\n' "${refresh_ids[@]}" | jq -s '.')

            local refresh_code
            refresh_code=$(curl -s --connect-timeout 10 --max-time 120 \
                -o "$body_file" -w "%{http_code}" \
                -X POST \
                -H "x-api-key: ${api_key}" \
                -H "Content-Type: application/json" \
                -d "{\"account_ids\": ${ids_json}}" \
                "${base_url}/api/v1/admin/accounts/batch-refresh")

            local refresh_body
            refresh_body=$(cat "$body_file")

            if [ "$refresh_code" = "200" ]; then
                local success_count failed_count
                success_count=$(echo "$refresh_body" | jq -r '.data.success_count // 0')
                failed_count=$(echo "$refresh_body" | jq -r '.data.failed_count // 0')
                echo "      ✅ 刷新完成: 成功=${success_count}, 失败=${failed_count}"
                refreshed=$((refreshed + success_count))
                errors=$((errors + failed_count))

                local errors_len
                errors_len=$(echo "$refresh_body" | jq '.data.errors | length // 0')
                if [ "$errors_len" -gt 0 ]; then
                    echo "      失败详情:"
                    echo "$refresh_body" | jq -r '.data.errors[]? | "        ID=\(.account_id) 错误=\(.error)"'
                fi
            else
                echo "      ❌ 批量刷新请求失败 (HTTP ${refresh_code})"
                echo "      响应: $(head -c 200 "$body_file" 2>/dev/null)"
                errors=$((errors + 1))
            fi
        fi

        if [ ${#delete_ids[@]} -eq 0 ] && [ ${#refresh_ids[@]} -eq 0 ]; then
            echo "   ✅ 没有需要处理的临时不可调度账号"
        fi
    } > "$log_file" 2>&1

    rm -f "$body_file"
    echo "${deleted} ${refreshed} ${errors}" > "$stats_file"
}

echo "=========================================="
echo "  临时不可调度账号处理脚本（并行版）"
echo "  $(date '+%Y-%m-%d %H:%M:%S')"
echo "=========================================="

# 收集所有服务器配置
declare -a server_names=()
declare -a server_urls=()
declare -a server_keys=()

while IFS='|' read -r name url key; do
    [[ "$name" =~ ^#.*$ ]] && continue
    [[ -z "$name" ]] && continue

    name=$(echo "$name" | xargs)
    url=$(echo "$url" | xargs)
    key=$(echo "$key" | xargs)

    if [ -z "$url" ] || [ -z "$key" ]; then
        echo "⚠️  跳过无效配置行: ${name}"
        continue
    fi

    server_names+=("$name")
    server_urls+=("$url")
    server_keys+=("$key")
done < "$CONF_FILE"

server_count=${#server_names[@]}
if [ "$server_count" -eq 0 ]; then
    echo "⚠️  配置文件中没有有效的服务器配置"
    exit 0
fi

echo "📡 共 ${server_count} 个号池，并行处理中..."

# 并行启动所有号池处理
pids=()
for ((idx=0; idx<server_count; idx++)); do
    log_file="${WORK_DIR}/log_${idx}.txt"
    stats_file="${WORK_DIR}/stats_${idx}.txt"
    process_server "${server_names[$idx]}" "${server_urls[$idx]}" "${server_keys[$idx]}" "$log_file" "$stats_file" &
    pids+=($!)
done

# 等待所有并行任务完成
for pid in "${pids[@]}"; do
    wait "$pid" 2>/dev/null || true
done

# 按顺序输出各号池日志
total_deleted=0
total_refreshed=0
total_errors=0

for ((idx=0; idx<server_count; idx++)); do
    log_file="${WORK_DIR}/log_${idx}.txt"
    stats_file="${WORK_DIR}/stats_${idx}.txt"

    [ -f "$log_file" ] && cat "$log_file"

    if [ -f "$stats_file" ]; then
        read -r d r e < "$stats_file"
        total_deleted=$((total_deleted + d))
        total_refreshed=$((total_refreshed + r))
        total_errors=$((total_errors + e))
    fi
done

echo ""
echo "=========================================="
echo "  全部处理完成"
echo "  删除: ${total_deleted} | 刷新: ${total_refreshed} | 错误: ${total_errors}"
echo "  $(date '+%Y-%m-%d %H:%M:%S')"
echo "=========================================="
