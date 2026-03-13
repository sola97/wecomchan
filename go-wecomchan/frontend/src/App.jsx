import { useEffect, useMemo, useState } from "react";

const MAX_IMAGE_BYTES = 2 * 1024 * 1024;
const JPEG_COMPRESSION_LEVELS = [
  { label: "高质量", quality: 0.92 },
  { label: "中等质量", quality: 0.78 },
  { label: "低质量", quality: 0.62 },
];

const EMPTY_CONFIG = {
  route_suffix: "",
  display_name: "",
  sendkey: "",
  wecom_cid: "",
  wecom_secret: "",
  wecom_aid: "",
  wecom_touid: "@all",
  callback_token: "",
  callback_aes_key: "",
};

const EMPTY_REDIS = {
  enabled: false,
  addr: "",
  password: "",
};

const DEFAULT_LOG_QUERY = {
  days: 7,
  limit: 200,
  routeSuffix: "",
};

const FALLBACK_DOCS = {
  routes: [
    {
      title: "GET 发送文本",
      method: "GET",
      path: "/<route_suffix>",
      description: "通过查询参数发送文本消息。",
      params: [
        { name: "sendkey", location: "query", required: "true", description: "发送密钥" },
        { name: "msg", location: "query", required: "true", description: "文本内容" },
        { name: "msg_type", location: "query", required: "true", description: "固定为 text" },
      ],
    },
    {
      title: "POST 发送文本/Markdown",
      method: "POST",
      path: "/<route_suffix>",
      description: "支持 application/json、x-www-form-urlencoded、multipart/form-data。",
      params: [
        { name: "sendkey", location: "body", required: "true", description: "发送密钥" },
        { name: "msg", location: "body", required: "true", description: "消息内容" },
        { name: "msg_type", location: "body", required: "true", description: "text 或 markdown" },
      ],
    },
    {
      title: "POST 图片发送",
      method: "POST",
      path: "/<route_suffix>",
      description: "使用 multipart/form-data 上传图片字段 media。",
      params: [
        { name: "sendkey", location: "body/query", required: "true", description: "发送密钥" },
        { name: "msg_type", location: "body/query", required: "true", description: "固定为 image" },
        { name: "media", location: "multipart", required: "true", description: "图片文件，最大 2MB" },
      ],
    },
    {
      title: "POST 图文双发",
      method: "POST",
      path: "/<route_suffix>",
      description: "沿用文本接口，增加 image 字段后自动拆成 text 与 image 两条消息发送。",
      params: [
        { name: "sendkey", location: "body", required: "true", description: "发送密钥" },
        { name: "msg", location: "body", required: "true", description: "文本内容" },
        { name: "msg_type", location: "body", required: "false", description: "可不传，传入时固定为 text" },
        { name: "image", location: "body", required: "true", description: "图片 base64 字符串" },
        { name: "filename", location: "body", required: "false", description: "图片文件名" },
      ],
    },
  ],
  verification_routes: [
    {
      title: "企业微信回调 URL 验证",
      method: "GET",
      path: "/<route_suffix>/callback",
      description: "企业微信配置回调地址时使用。",
      params: [
        { name: "msg_signature", location: "query", required: "true", description: "消息签名" },
        { name: "timestamp", location: "query", required: "true", description: "时间戳" },
        { name: "nonce", location: "query", required: "true", description: "随机串" },
        { name: "echostr", location: "query", required: "true", description: "企业微信下发的随机字符串" },
      ],
    },
    {
      title: "企业微信回调消息接收",
      method: "POST",
      path: "/<route_suffix>/callback",
      description: "当前服务保留该接口入口，可用于后续扩展消息回调处理。",
      params: [
        { name: "msg_signature", location: "query", required: "true", description: "消息签名" },
        { name: "timestamp", location: "query", required: "true", description: "时间戳" },
        { name: "nonce", location: "query", required: "true", description: "随机串" },
        { name: "body", location: "body", required: "true", description: "企业微信 XML 加密消息体" },
      ],
    },
  ],
};

const TOOL_TITLES = {
  getText: "GET 发送文本",
  postMessage: "POST 发送文本/Markdown",
  image: "POST 图片发送",
  textImage: "POST 图文双发",
};

const ADMIN_PAGES = {
  config: "config",
  redis: "redis",
  tools: "tools",
  logs: "logs",
};

const PAGE_META = {
  [ADMIN_PAGES.config]: {
    title: "机器人配置",
    description: "维护不同机器人路由，并在左侧点选时自动把右侧表单切到对应配置。",
  },
  [ADMIN_PAGES.redis]: {
    title: "Redis 配置",
    description: "统一维护全局 Redis 地址和密码。这个配置较少修改，单独放在一页。",
  },
  [ADMIN_PAGES.tools]: {
    title: "接口测试",
    description: "按路由切换测试目标，查看发送模板、执行结果和 callback 参数说明。",
  },
  [ADMIN_PAGES.logs]: {
    title: "发送日志",
    description: "查看最近 7 天发送记录，并按路由筛选文本和图片消息。",
  },
};

const NAV_ITEMS = [
  {
    key: ADMIN_PAGES.config,
    label: "机器人配置",
    note: "新增路由、维护企业微信凭证与回调配置。",
  },
  {
    key: ADMIN_PAGES.redis,
    label: "Redis 配置",
    note: "维护全局缓存地址、密码和开关。",
  },
  {
    key: ADMIN_PAGES.tools,
    label: "接口测试",
    note: "按路由执行文本、图片和图文消息调试。",
  },
  {
    key: ADMIN_PAGES.logs,
    label: "发送日志",
    note: "查看最近 7 天的消息发送记录。",
  },
];

function detectAdminPage() {
  if (typeof window === "undefined") {
    return ADMIN_PAGES.config;
  }

  const path = window.location.pathname.replace(/^\/admin\/?/, "");
  if (path.startsWith("redis")) {
    return ADMIN_PAGES.redis;
  }
  if (path.startsWith("tools")) {
    return ADMIN_PAGES.tools;
  }
  if (path.startsWith("logs")) {
    return ADMIN_PAGES.logs;
  }
  return ADMIN_PAGES.config;
}

function adminPagePath(page) {
  switch (page) {
    case ADMIN_PAGES.redis:
      return "/admin/redis";
    case ADMIN_PAGES.tools:
      return "/admin/tools";
    case ADMIN_PAGES.logs:
      return "/admin/logs";
    case ADMIN_PAGES.config:
    default:
      return "/admin/config";
  }
}

function stringify(value) {
  if (value == null) {
    return "";
  }
  if (typeof value === "string") {
    return value;
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function extractErrorMessage(payload, fallback) {
  if (!payload) {
    return fallback;
  }
  if (typeof payload === "string") {
    return payload;
  }
  if (typeof payload.error === "string") {
    return payload.error;
  }
  return fallback;
}

async function readJSON(response) {
  const contentType = response.headers.get("content-type") || "";
  if (contentType.includes("application/json")) {
    return response.json();
  }
  return response.text();
}

async function requestJSON(path, options = {}) {
  const response = await fetch(path, {
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      ...(options.headers || {}),
    },
    ...options,
  });

  const payload = await readJSON(response);
  if (!response.ok) {
    throw new Error(extractErrorMessage(payload, `Request failed (${response.status})`));
  }
  return payload;
}

function bytesToBase64(bytes) {
  if (!bytes || bytes.length === 0) {
    return "";
  }

  const chunkSize = 0x8000;
  let binary = "";
  for (let index = 0; index < bytes.length; index += chunkSize) {
    const chunk = bytes.subarray(index, index + chunkSize);
    binary += String.fromCharCode(...chunk);
  }
  return window.btoa(binary);
}

function routePath(routeSuffix) {
  return routeSuffix ? `/${routeSuffix}` : "/<route_suffix>";
}

function callbackPath(routeSuffix) {
  return routeSuffix ? `/${routeSuffix}/callback` : "/<route_suffix>/callback";
}

function normalizeConfigPayload(form) {
  return {
    route_suffix: form.route_suffix.trim(),
    display_name: form.display_name.trim(),
    sendkey: form.sendkey.trim(),
    wecom_cid: form.wecom_cid.trim(),
    wecom_secret: form.wecom_secret.trim(),
    wecom_aid: form.wecom_aid.trim(),
    wecom_touid: form.wecom_touid.trim(),
    callback_token: form.callback_token.trim(),
    callback_aes_key: form.callback_aes_key.trim(),
  };
}

function normalizeRedisPayload(form) {
  return {
    enabled: Boolean(form?.enabled),
    addr: (form?.addr || "").trim(),
    password: (form?.password || "").trim(),
  };
}

function normalizeLogQuery(query) {
  return {
    days: Math.max(1, Number(query?.days || DEFAULT_LOG_QUERY.days)),
    limit: Math.max(1, Number(query?.limit || DEFAULT_LOG_QUERY.limit)),
    routeSuffix: (query?.routeSuffix || "").trim(),
  };
}

function formatBytes(bytes) {
  const value = Number(bytes || 0);
  if (!Number.isFinite(value) || value <= 0) {
    return "0 B";
  }
  if (value >= 1024 * 1024) {
    return `${(value / (1024 * 1024)).toFixed(2)} MB`;
  }
  if (value >= 1024) {
    return `${Math.round(value / 1024)} KB`;
  }
  return `${value} B`;
}

function formatDateTime(value) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
}

function replaceFileExtension(filename, extension) {
  const safeName = (filename || "upload-image").trim() || "upload-image";
  const normalizedExt = extension.startsWith(".") ? extension : `.${extension}`;
  const baseName = safeName.replace(/\.[^.]+$/, "");
  return `${baseName}${normalizedExt}`;
}

function fileToImage(file) {
  return new Promise((resolve, reject) => {
    const objectUrl = URL.createObjectURL(file);
    const image = new Image();
    image.onload = () => {
      URL.revokeObjectURL(objectUrl);
      resolve(image);
    };
    image.onerror = () => {
      URL.revokeObjectURL(objectUrl);
      reject(new Error("图片读取失败，无法进行压缩。"));
    };
    image.src = objectUrl;
  });
}

function canvasToBlob(canvas, mimeType, quality) {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error("图片压缩失败，浏览器没有返回可用文件。"));
        return;
      }
      resolve(blob);
    }, mimeType, quality);
  });
}

async function prepareImageFile(file) {
  if (!file) {
    throw new Error("请先选择一张图片文件。");
  }
  if (!file.type.startsWith("image/")) {
    throw new Error("所选文件不是图片。");
  }
  if (file.size <= MAX_IMAGE_BYTES) {
    return {
      file,
      compressed: false,
      originalSize: file.size,
      finalSize: file.size,
      strategy: "原图",
      quality: null,
    };
  }

  const image = await fileToImage(file);
  const width = image.naturalWidth || image.width;
  const height = image.naturalHeight || image.height;
  if (!width || !height) {
    throw new Error("无法识别图片尺寸，压缩失败。");
  }

  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;

  const context = canvas.getContext("2d");
  if (!context) {
    throw new Error("当前浏览器不支持图片压缩。");
  }

  context.fillStyle = "#ffffff";
  context.fillRect(0, 0, width, height);
  context.drawImage(image, 0, 0, width, height);

  let smallestResult = null;
  for (const level of JPEG_COMPRESSION_LEVELS) {
    const blob = await canvasToBlob(canvas, "image/jpeg", level.quality);
    const nextFile = new File([blob], replaceFileExtension(file.name, ".jpg"), {
      type: "image/jpeg",
      lastModified: Date.now(),
    });
    const nextResult = {
      file: nextFile,
      compressed: true,
      originalSize: file.size,
      finalSize: blob.size,
      strategy: level.label,
      quality: level.quality,
    };

    if (!smallestResult || nextResult.finalSize < smallestResult.finalSize) {
      smallestResult = nextResult;
    }
    if (blob.size <= MAX_IMAGE_BYTES) {
      return nextResult;
    }
  }

  if (smallestResult) {
    throw new Error(
      `图片压缩到低质量后仍为 ${formatBytes(smallestResult.finalSize)}，超过 2MB 限制，请换一张更小的图片。`,
    );
  }
  throw new Error("图片压缩失败，请稍后重试。");
}

function decorateImageExecution(payload, sourceFile, preparedFile, extra = {}) {
  const prepared = preparedFile || sourceFile;
  return {
    ...payload,
    request: {
      ...(payload?.request || {}),
      image_prepare: {
        original_name: sourceFile?.name || "",
        original_size_bytes: sourceFile?.size || 0,
        original_size: formatBytes(sourceFile?.size || 0),
        upload_name: prepared?.name || "",
        upload_size_bytes: prepared?.size || 0,
        upload_size: formatBytes(prepared?.size || 0),
        compressed: Boolean(sourceFile && prepared && sourceFile !== prepared),
        quality_profile: extra.strategy || "原图",
        jpeg_quality: extra.quality ?? null,
        ...extra,
      },
    },
  };
}

function findRouteDoc(docState, title) {
  return (
    docState.routes?.find((item) => item.title === title) ||
    FALLBACK_DOCS.routes.find((item) => item.title === title)
  );
}

function formatDocUrlForBrowser(path, browserOrigin) {
  if (!path) {
    return "";
  }
  if (!browserOrigin) {
    return path;
  }

  try {
    const resolved = new URL(path, browserOrigin);
    return `${browserOrigin}${resolved.pathname}${resolved.search}${resolved.hash}`;
  } catch {
    return path;
  }
}

function CodeBlock({ title, children }) {
  return (
    <div className="rounded-[18px] border border-slate-800 bg-[#0f1720] p-4 text-sm text-slate-100 shadow-inner">
      <div className="mb-2 text-xs font-semibold uppercase tracking-[0.24em] text-slate-400">{title}</div>
      <pre className="overflow-x-auto whitespace-pre-wrap break-all font-mono text-[13px] leading-6">{children}</pre>
    </div>
  );
}

function ParamTable({ params }) {
  if (!params?.length) {
    return null;
  }

  return (
    <div className="overflow-hidden rounded-[18px] border border-[#e7ddd0] bg-[#fcfbf8]">
      <table className="min-w-full divide-y divide-[#ebe2d6] text-left text-sm">
        <thead className="bg-[#f5efe6] text-slate-500">
          <tr>
            <th className="px-4 py-3 font-medium">参数</th>
            <th className="px-4 py-3 font-medium">位置</th>
            <th className="px-4 py-3 font-medium">必填</th>
            <th className="px-4 py-3 font-medium">说明</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-[#efe6da] text-slate-700">
          {params.map((param) => (
            <tr key={`${param.name}-${param.location}`}>
              <td className="px-4 py-3 font-mono text-xs text-slate-900">{param.name}</td>
              <td className="px-4 py-3">{param.location}</td>
              <td className="px-4 py-3">{param.required}</td>
              <td className="px-4 py-3">{param.description}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Field({ label, children, hint, dark = false }) {
  return (
    <label className="flex flex-col gap-2">
      <span className={`text-sm font-semibold ${dark ? "text-slate-100" : "text-slate-800"}`}>{label}</span>
      {children}
      {hint ? <span className={`text-xs ${dark ? "text-slate-400" : "text-slate-500"}`}>{hint}</span> : null}
    </label>
  );
}

function ToolCard({ badge, title, description, endpointMethod, endpointPath, template, params, children, execution }) {
  return (
    <section className="card-panel overflow-hidden !p-0">
      <div className="flex flex-col gap-4 border-b border-[#ede3d7] px-6 py-6 md:flex-row md:items-start md:justify-between">
        <div className="space-y-2">
          <div className="inline-flex rounded-full border border-amber-200 bg-amber-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.22em] text-amber-700">
            {badge}
          </div>
          <div>
            <h2 className="text-2xl font-semibold tracking-tight text-slate-950">{title}</h2>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-600">{description}</p>
          </div>
        </div>
        <div className="subtle-panel px-4 py-3 text-sm text-slate-600">
          <div className="text-xs uppercase tracking-[0.2em] text-slate-400">Endpoint</div>
          <div className="mt-2 flex flex-wrap items-center gap-3">
            <span className="inline-flex rounded-full border border-amber-200 bg-amber-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.22em] text-amber-700">
              {endpointMethod}
            </span>
            <span className="font-mono text-[13px] text-slate-900">{endpointPath}</span>
          </div>
        </div>
      </div>

      <div className="grid gap-5 px-6 py-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(300px,0.9fr)]">
        <div className="space-y-5">
          <CodeBlock title="模板">{template}</CodeBlock>
          <ParamTable params={params} />
          <div className="subtle-panel p-5">{children}</div>
        </div>
        <div className="space-y-4">
          <CodeBlock title="执行请求">
            {execution?.request ? stringify(execution.request) : "点击执行后显示最近一次请求模板。"}
          </CodeBlock>
          <CodeBlock title="执行结果">
            {execution?.result ? stringify(execution.result) : execution?.error || "执行后这里会展示接口返回结果。"}
          </CodeBlock>
        </div>
      </div>
    </section>
  );
}

export default function App() {
  const [session, setSession] = useState({
    loading: true,
    configured: true,
    authenticated: false,
    distAvailable: true,
    configCount: 0,
    defaultRoute: "wecomchan",
    configStorePath: "",
  });
  const [page, setPage] = useState(detectAdminPage);
  const [docs, setDocs] = useState(FALLBACK_DOCS);
  const [configs, setConfigs] = useState([]);
  const [selectedRoute, setSelectedRoute] = useState("");
  const [loginPassword, setLoginPassword] = useState("");
  const [loginError, setLoginError] = useState("");
  const [configMode, setConfigMode] = useState("create");
  const [editingRoute, setEditingRoute] = useState("");
  const [configForm, setConfigForm] = useState(EMPTY_CONFIG);
  const [configFeedback, setConfigFeedback] = useState("");
  const [redisForm, setRedisForm] = useState(EMPTY_REDIS);
  const [redisFeedback, setRedisFeedback] = useState("");
  const [messageLogs, setMessageLogs] = useState([]);
  const [logQuery, setLogQuery] = useState(DEFAULT_LOG_QUERY);
  const [logFeedback, setLogFeedback] = useState("");
  const [logError, setLogError] = useState("");
  const [busy, setBusy] = useState({
    login: false,
    loadConfigs: false,
    saveConfig: false,
    saveRedis: false,
    deleteConfig: false,
    loadLogs: false,
    getText: false,
    postMessage: false,
    image: false,
    textImage: false,
  });
  const [results, setResults] = useState({});
  const [forms, setForms] = useState({
    getTextMessage: "hello from admin console",
    postMessageType: "markdown",
    postMessageBody: "## Wecomchan\n\n这是一条来自管理台的 markdown 测试消息。",
    imageFile: null,
    textImageMessage: "这是一条先文本后图片的图文消息。",
    textImageFile: null,
  });

  useEffect(() => {
    void bootstrap();
  }, []);

  useEffect(() => {
    const handlePopState = () => {
      setPage(detectAdminPage());
    };

    window.addEventListener("popstate", handlePopState);
    return () => {
      window.removeEventListener("popstate", handlePopState);
    };
  }, []);

  useEffect(() => {
    if (!session.authenticated || !selectedRoute) {
      return;
    }
    void loadDocs(selectedRoute);
  }, [session.authenticated, selectedRoute]);

  useEffect(() => {
    if (!session.authenticated || page !== ADMIN_PAGES.logs) {
      return;
    }
    void loadMessageLogs();
  }, [session.authenticated, page, logQuery.days, logQuery.limit, logQuery.routeSuffix]);

  const selectedConfig = useMemo(
    () => configs.find((item) => item.route_suffix === selectedRoute) || null,
    [configs, selectedRoute],
  );

  const editingConfig = useMemo(
    () => configs.find((item) => item.route_suffix === editingRoute) || null,
    [configs, editingRoute],
  );

  const pageMeta = PAGE_META[page] || PAGE_META[ADMIN_PAGES.config];
  const getTextDoc = findRouteDoc(docs, TOOL_TITLES.getText);
  const postDoc = findRouteDoc(docs, TOOL_TITLES.postMessage);
  const imageDoc = findRouteDoc(docs, TOOL_TITLES.image);
  const textImageDoc = findRouteDoc(docs, TOOL_TITLES.textImage);

  async function bootstrap() {
    try {
      const currentSession = await requestJSON("/api/admin/session");
      setSession({
        loading: false,
        configured: Boolean(currentSession.configured ?? true),
        authenticated: Boolean(currentSession.authenticated),
        distAvailable: Boolean(currentSession.dist_available ?? true),
        configCount: Number(currentSession.config_count ?? 0),
        defaultRoute: currentSession.default_route || "wecomchan",
        configStorePath: currentSession.config_store_path || "",
      });
      setRedisForm(normalizeRedisPayload(currentSession.redis));

      if (currentSession.authenticated) {
        await loadConfigs();
      }
    } catch (error) {
      setSession((current) => ({
        ...current,
        loading: false,
        configured: true,
        authenticated: false,
      }));
      setLoginError(error.message);
    }
  }

  async function loadConfigs(preferredRoute = "") {
    updateBusy("loadConfigs", true);
    try {
      const payload = await requestJSON("/api/admin/configs");
      const configList = Array.isArray(payload.configs) ? payload.configs : [];
      const defaultRoute = payload.default_route_suffix || session.defaultRoute || "wecomchan";

      setConfigs(configList);
      setRedisForm(normalizeRedisPayload(payload.redis));
      setSession((current) => ({
        ...current,
        configCount: configList.length,
        defaultRoute,
      }));

      setLogQuery((current) => {
        if (!current.routeSuffix) {
          return current;
        }
        const routeStillExists = configList.some((item) => item.route_suffix === current.routeSuffix);
        return routeStillExists ? current : { ...current, routeSuffix: "" };
      });

      if (configList.length === 0) {
        setSelectedRoute("");
        setDocs(FALLBACK_DOCS);
        setConfigMode("create");
        setEditingRoute("");
        setConfigForm({
          ...EMPTY_CONFIG,
          route_suffix: defaultRoute,
        });
        return;
      }

      const nextRoute =
        preferredRoute ||
        editingRoute ||
        selectedRoute ||
        defaultRoute ||
        configList[0]?.route_suffix ||
        "";
      const matched = configList.find((item) => item.route_suffix === nextRoute) || configList[0];
      setSelectedRoute(matched.route_suffix);

      if (configMode === "edit") {
        const nextEditing =
          configList.find((item) => item.route_suffix === editingRoute) || matched;
        setEditingRoute(nextEditing.route_suffix);
        setConfigForm({
          ...EMPTY_CONFIG,
          ...nextEditing,
        });
      } else {
        setConfigMode("edit");
        setEditingRoute(matched.route_suffix);
        setConfigForm({
          ...EMPTY_CONFIG,
          ...matched,
        });
      }
    } catch (error) {
      setConfigFeedback(error.message);
    } finally {
      updateBusy("loadConfigs", false);
    }
  }

  async function loadDocs(routeSuffix) {
    try {
      const query = routeSuffix ? `?route_suffix=${encodeURIComponent(routeSuffix)}` : "";
      const payload = await requestJSON(`/api/admin/docs${query}`);
      setDocs({
        routes: Array.isArray(payload.routes) ? payload.routes : FALLBACK_DOCS.routes,
        verification_routes: Array.isArray(payload.verification_routes)
          ? payload.verification_routes
          : FALLBACK_DOCS.verification_routes,
      });
    } catch (error) {
      setDocs(FALLBACK_DOCS);
      setResults((current) => ({
        ...current,
        docs: { error: error.message },
      }));
    }
  }

  async function loadMessageLogs() {
    updateBusy("loadLogs", true);
    setLogError("");
    try {
      const query = normalizeLogQuery(logQuery);
      const params = new URLSearchParams({
        days: String(query.days),
        limit: String(query.limit),
      });
      if (query.routeSuffix) {
        params.set("route_suffix", query.routeSuffix);
      }
      const payload = await requestJSON(`/api/admin/logs/messages?${params.toString()}`);
      const items = Array.isArray(payload.items) ? payload.items : [];
      setMessageLogs(items);
      setLogFeedback(
        `已加载最近 ${payload.days || query.days} 天内的 ${items.length} 条发送记录。`,
      );
    } catch (error) {
      setMessageLogs([]);
      setLogFeedback("");
      setLogError(error.message);
    } finally {
      updateBusy("loadLogs", false);
    }
  }

  function updateBusy(key, value) {
    setBusy((current) => ({ ...current, [key]: value }));
  }

  function setExecution(key, payload) {
    setResults((current) => ({ ...current, [key]: payload }));
  }

  function updateForm(key, value) {
    setForms((current) => ({ ...current, [key]: value }));
  }

  function updateConfigForm(key, value) {
    setConfigForm((current) => ({ ...current, [key]: value }));
  }

  function updateRedisForm(key, value) {
    setRedisForm((current) => ({ ...current, [key]: value }));
  }

  function updateLogQuery(key, value) {
    setLogQuery((current) => normalizeLogQuery({ ...current, [key]: value }));
  }

  function navigateToPage(nextPage) {
    const targetPath = adminPagePath(nextPage);
    if (window.location.pathname !== targetPath) {
      window.history.pushState({}, "", targetPath);
    }
    setPage(nextPage);
  }

  function beginCreateConfig() {
    setConfigMode("create");
    setEditingRoute("");
    setConfigFeedback("");
    setConfigForm({
      ...EMPTY_CONFIG,
      route_suffix: session.defaultRoute || "wecomchan",
    });
  }

  function beginEditConfig(config) {
    setConfigMode("edit");
    setEditingRoute(config.route_suffix);
    setConfigFeedback("");
    setConfigForm({
      ...EMPTY_CONFIG,
      ...config,
    });
    setSelectedRoute(config.route_suffix);
  }

  async function handleLogin(event) {
    event.preventDefault();
    setLoginError("");
    updateBusy("login", true);

    try {
      await requestJSON("/api/admin/login", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ password: loginPassword }),
      });

      setLoginPassword("");
      setSession((current) => ({ ...current, authenticated: true }));
      await loadConfigs();
    } catch (error) {
      setLoginError(error.message);
    } finally {
      updateBusy("login", false);
    }
  }

  async function handleLogout() {
    await requestJSON("/api/admin/logout", { method: "POST" });
    setSession((current) => ({ ...current, authenticated: false }));
    setResults({});
    setConfigs([]);
    setSelectedRoute("");
    setEditingRoute("");
    setMessageLogs([]);
  }

  async function handleSaveConfig(event) {
    event.preventDefault();
    updateBusy("saveConfig", true);
    setConfigFeedback("");

    try {
      const payload = normalizeConfigPayload(configForm);
      const method = configMode === "edit" ? "PUT" : "POST";
      const path =
        configMode === "edit"
          ? `/api/admin/configs/${encodeURIComponent(editingRoute)}`
          : "/api/admin/configs";

      const response = await requestJSON(path, {
        method,
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
      });

      const savedConfig = response.config;
      setConfigFeedback(`配置 ${savedConfig.route_suffix} 已保存`);
      setConfigMode("edit");
      setEditingRoute(savedConfig.route_suffix);
      setConfigForm({
        ...EMPTY_CONFIG,
        ...savedConfig,
      });
      await loadConfigs(savedConfig.route_suffix);
    } catch (error) {
      setConfigFeedback(error.message);
    } finally {
      updateBusy("saveConfig", false);
    }
  }

  async function handleSaveRedis(event) {
    event.preventDefault();
    updateBusy("saveRedis", true);
    setRedisFeedback("");

    try {
      const payload = normalizeRedisPayload(redisForm);
      const response = await requestJSON("/api/admin/settings/redis", {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
      });
      setRedisForm(normalizeRedisPayload(response.redis));
      setRedisFeedback("全局 Redis 配置已保存");
    } catch (error) {
      setRedisFeedback(error.message);
    } finally {
      updateBusy("saveRedis", false);
    }
  }

  async function handleDeleteConfig(routeSuffix) {
    if (!window.confirm(`确定删除配置 ${routeSuffix} 吗？`)) {
      return;
    }

    updateBusy("deleteConfig", true);
    setConfigFeedback("");
    try {
      await requestJSON(`/api/admin/configs/${encodeURIComponent(routeSuffix)}`, {
        method: "DELETE",
      });
      setConfigFeedback(`配置 ${routeSuffix} 已删除`);
      const remaining = configs.filter((item) => item.route_suffix !== routeSuffix);
      const nextRoute = remaining[0]?.route_suffix || "";
      await loadConfigs(nextRoute);
    } catch (error) {
      setConfigFeedback(error.message);
    } finally {
      updateBusy("deleteConfig", false);
    }
  }

  async function executeGetText() {
    if (!selectedRoute) {
      setExecution("getText", { error: "请先选择一个路由配置。" });
      return;
    }
    updateBusy("getText", true);
    try {
      const payload = await requestJSON("/api/admin/execute/get-text", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          route_suffix: selectedRoute,
          msg: forms.getTextMessage,
        }),
      });
      setExecution("getText", payload);
    } catch (error) {
      setExecution("getText", { error: error.message });
    } finally {
      updateBusy("getText", false);
    }
  }

  async function executePostMessage() {
    if (!selectedRoute) {
      setExecution("postMessage", { error: "请先选择一个路由配置。" });
      return;
    }
    updateBusy("postMessage", true);
    try {
      const payload = await requestJSON("/api/admin/execute/post-message", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          route_suffix: selectedRoute,
          msg: forms.postMessageBody,
          msg_type: forms.postMessageType,
        }),
      });
      setExecution("postMessage", payload);
    } catch (error) {
      setExecution("postMessage", { error: error.message });
    } finally {
      updateBusy("postMessage", false);
    }
  }

  async function executeImage() {
    if (!selectedRoute) {
      setExecution("image", { error: "请先选择一个路由配置。" });
      return;
    }
    if (!forms.imageFile) {
      setExecution("image", { error: "请先选择一张图片文件。" });
      return;
    }

    updateBusy("image", true);
    try {
      const prepared = await prepareImageFile(forms.imageFile);
      const body = new FormData();
      body.append("route_suffix", selectedRoute);
      body.append("media", prepared.file);
      body.append("msg_type", "image");

      const payload = await requestJSON("/api/admin/execute/image", {
        method: "POST",
        body,
      });
      setExecution(
        "image",
        decorateImageExecution(payload, forms.imageFile, prepared.file, {
          strategy: prepared.strategy,
          quality: prepared.quality,
        }),
      );
    } catch (error) {
      setExecution("image", { error: error.message });
    } finally {
      updateBusy("image", false);
    }
  }

  async function executeTextImage() {
    if (!selectedRoute) {
      setExecution("textImage", { error: "请先选择一个路由配置。" });
      return;
    }
    if (!forms.textImageFile) {
      setExecution("textImage", { error: "请先选择一张图片文件。" });
      return;
    }

    updateBusy("textImage", true);
    try {
      const prepared = await prepareImageFile(forms.textImageFile);
      const bytes = new Uint8Array(await prepared.file.arrayBuffer());
      const imageBase64 = bytesToBase64(bytes);
      const payload = await requestJSON("/api/admin/execute/base62", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          route_suffix: selectedRoute,
          msg: forms.textImageMessage,
          msg_type: "text",
          image: imageBase64,
          filename: prepared.file.name,
        }),
      });
      setExecution(
        "textImage",
        decorateImageExecution(payload, forms.textImageFile, prepared.file, {
          strategy: prepared.strategy,
          quality: prepared.quality,
          base64_length: imageBase64.length,
        }),
      );
    } catch (error) {
      setExecution("textImage", { error: error.message });
    } finally {
      updateBusy("textImage", false);
    }
  }

  const selectedRoutePath = routePath(selectedRoute);
  const selectedCallbackPath = callbackPath(selectedRoute);
  const browserOrigin = typeof window === "undefined" ? "" : window.location.origin;
  const callbackPreviewUrl = `${browserOrigin}${callbackPath(configForm.route_suffix.trim())}`;
  const getTextEndpointPath = formatDocUrlForBrowser(getTextDoc.path, browserOrigin);
  const postEndpointPath = formatDocUrlForBrowser(postDoc.path, browserOrigin);
  const imageEndpointPath = formatDocUrlForBrowser(imageDoc.path, browserOrigin);
  const textImageEndpointPath = formatDocUrlForBrowser(textImageDoc.path, browserOrigin);

  const getTemplate = useMemo(
    () => `GET ${selectedRoutePath}?sendkey=<SENDKEY>&msg=hello&msg_type=text`,
    [selectedRoutePath],
  );
  const postTemplate = useMemo(
    () =>
      `{\n  "sendkey": "<SENDKEY>",\n  "msg": "markdown or text body",\n  "msg_type": "${forms.postMessageType}"\n}`,
    [forms.postMessageType],
  );
  const imageTemplate = useMemo(
    () =>
      `multipart/form-data:\n- sendkey=<SENDKEY>\n- msg_type=image\n- media=@/${forms.imageFile?.name || "path/to/image.png"}\n\n如果图片超过 2MB，管理台会按高质量 -> 中等质量 -> 低质量 JPEG 自动压缩。`,
    [forms.imageFile],
  );
  const textImageTemplate = useMemo(
    () =>
      '{\n  "sendkey": "<SENDKEY>",\n  "msg": "text content",\n  "msg_type": "text",\n  "image": "<BASE64_IMAGE_DATA>",\n  "filename": "image.jpg"\n}\n\n如果图片超过 2MB，管理台会先压缩再转 base64。',
    [],
  );

  if (session.loading) {
    return (
      <main className="min-h-screen bg-stone-100 px-6 py-16">
        <div className="mx-auto max-w-5xl rounded-xl border border-stone-200 bg-white/85 p-10 shadow-panel backdrop-blur">
          <div className="text-sm uppercase tracking-[0.28em] text-amber-700">Wecomchan Admin</div>
          <h1 className="mt-4 text-4xl font-semibold tracking-tight text-slate-950">正在加载管理台状态</h1>
          <p className="mt-4 text-slate-600">正在检查登录态和配置存储。</p>
        </div>
      </main>
    );
  }

  if (!session.authenticated) {
    return (
      <main className="relative min-h-screen overflow-hidden px-4 py-4 sm:px-6 lg:px-8">
        <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(245,158,11,0.18),transparent_24%),radial-gradient(circle_at_80%_12%,rgba(16,185,129,0.14),transparent_20%),radial-gradient(circle_at_45%_78%,rgba(59,130,246,0.08),transparent_18%)]" />

        <div className="relative mx-auto grid max-w-[1360px] gap-5 lg:grid-cols-[minmax(0,1.2fr)_420px]">
          <section className="card-panel overflow-hidden !p-0">
            <div className="border-b border-[#ece2d6] px-6 py-4 text-xs font-semibold uppercase tracking-[0.28em] text-clay">
              Wecomchan Admin
            </div>
            <div className="grid gap-8 px-6 py-8 lg:grid-cols-[minmax(0,1.2fr)_minmax(260px,0.8fr)]">
              <div>
                <div className="section-chip">Multi Bot Console</div>
                <h1 className="mt-6 max-w-3xl text-4xl font-semibold tracking-tight text-ink lg:text-5xl">
                  同一个服务里，集中管理多个企业微信机器人路由。
                </h1>
                <p className="mt-6 max-w-2xl text-base leading-8 text-slate-600">
                  登录后可以维护不同路由后缀、企业 ID、AgentId、Secret、接收消息服务器配置，并直接在后台完成接口调试、Redis 配置和最近 7 天发送日志查看。
                </p>
              </div>

              <div className="grid gap-4 self-start">
                <div className="metric-tile">
                  <div className="text-xs uppercase tracking-[0.22em] text-slate-400">配置文件</div>
                  <div className="mt-2 break-all font-mono text-sm text-slate-900">{session.configStorePath || "./data/bot-config.json"}</div>
                </div>
                <div className="metric-tile">
                  <div className="text-xs uppercase tracking-[0.22em] text-slate-400">登录方式</div>
                  <div className="mt-2 text-sm font-medium text-slate-900">环境变量 `WEB_PASSWORD`</div>
                </div>
                <div className="metric-tile">
                  <div className="text-xs uppercase tracking-[0.22em] text-slate-400">界面能力</div>
                  <div className="mt-2 text-sm leading-7 text-slate-700">配置管理、接口测试、日志查看、Redis 全局配置。</div>
                </div>
              </div>
            </div>
          </section>

          <section className="card-panel flex flex-col justify-between bg-slate-950 text-white">
            <div>
              <div className="text-xs font-semibold uppercase tracking-[0.28em] text-slate-400">Admin Login</div>
              <h2 className="mt-4 text-3xl font-semibold tracking-tight text-white">输入管理密码</h2>
              <p className="mt-3 text-sm leading-7 text-slate-300">
                {session.configured
                  ? "登录成功后会加载机器人配置、Redis 设置、接口文档、测试工具和最近 7 天发送日志。"
                  : "当前服务还没有配置 WEB_PASSWORD，请先设置环境变量后再访问。"}
              </p>
            </div>

            <form className="mt-8 space-y-5" onSubmit={handleLogin}>
              <Field label="管理密码" hint="后端通过 WEB_PASSWORD 校验。" dark>
                <input
                  type="password"
                  value={loginPassword}
                  onChange={(event) => setLoginPassword(event.target.value)}
                  className="input-shell border-slate-700 bg-slate-900 text-white placeholder:text-slate-500"
                  placeholder="请输入 WEB_PASSWORD"
                  disabled={!session.configured || busy.login}
                />
              </Field>

              {loginError ? <div className="rounded-xl border border-rose-700/50 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">{loginError}</div> : null}

              <button type="submit" className="primary-button w-full bg-white text-slate-950 hover:bg-stone-100" disabled={!session.configured || busy.login}>
                {busy.login ? "登录中..." : "进入管理台"}
              </button>
            </form>
          </section>
        </div>
      </main>
    );
  }

  return (
    <main className="relative min-h-screen overflow-hidden px-4 py-4 sm:px-6 lg:px-8">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(245,158,11,0.16),transparent_26%),radial-gradient(circle_at_82%_14%,rgba(16,185,129,0.12),transparent_18%),radial-gradient(circle_at_65%_75%,rgba(59,130,246,0.08),transparent_18%)]" />

      <div className="relative mx-auto max-w-[1450px] space-y-5">
        <header className="card-panel flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
          <div className="min-w-0">
            <div className="text-xs font-semibold uppercase tracking-[0.28em] text-clay">Wecomchan Control Center</div>
            <div className="mt-2 text-lg font-semibold tracking-tight text-slate-950">多机器人管理、调试与消息回调工作台</div>
            <div className="mt-3 flex flex-wrap gap-2">
              <div className="section-chip">JSON 本地持久化</div>
              <div className="section-chip">动态路由</div>
              <div className="section-chip">接口测试 + 发送日志</div>
            </div>
          </div>

          <div className="flex flex-col gap-3 xl:min-w-[760px] xl:flex-row xl:items-center xl:justify-end">
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <div className="metric-tile">
                <div className="text-xs uppercase tracking-[0.22em] text-slate-400">当前路由</div>
                <div className="mt-2 font-mono text-sm text-slate-900">{selectedRoute ? `/${selectedRoute}` : "未选择"}</div>
              </div>
              <div className="metric-tile">
                <div className="text-xs uppercase tracking-[0.22em] text-slate-400">配置数量</div>
                <div className="mt-2 text-lg font-semibold text-slate-950">{configs.length}</div>
              </div>
              <div className="metric-tile">
                <div className="text-xs uppercase tracking-[0.22em] text-slate-400">默认路由</div>
                <div className="mt-2 font-mono text-sm text-slate-900">/{session.defaultRoute || "wecomchan"}</div>
              </div>
              <div className="metric-tile">
                <div className="text-xs uppercase tracking-[0.22em] text-slate-400">静态资源</div>
                <div className="mt-2 text-sm text-slate-900">{session.distAvailable ? "dist 已挂载" : "dist 未提供"}</div>
              </div>
            </div>

            <button className="primary-button xl:self-stretch" onClick={handleLogout}>
              退出登录
            </button>
          </div>
        </header>

        <div className="grid gap-5 xl:grid-cols-[220px_minmax(0,1fr)]">
          <aside className="card-panel overflow-hidden !p-0">
            <div className="border-b border-[#ece2d6] px-4 py-4">
              <div className="text-xs font-semibold uppercase tracking-[0.28em] text-clay">Navigation</div>
              <div className="mt-2 text-base font-semibold tracking-tight text-slate-950">工作区导航</div>
              <p className="mt-1 text-xs leading-5 text-slate-600">页面切换和运行状态统一放在左侧。</p>
            </div>

            <div className="p-2">
              {NAV_ITEMS.map((item) => {
                const active = page === item.key;
                return (
                  <button
                    key={item.key}
                    type="button"
                    className={`nav-button ${active ? "nav-button-active" : ""}`}
                    onClick={() => navigateToPage(item.key)}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <div className={`text-[13px] font-semibold ${active ? "text-slate-950" : "text-slate-800"}`}>{item.label}</div>
                        <div className={`mt-1 text-[11px] leading-5 ${active ? "text-slate-700" : "text-slate-500"}`}>{item.note}</div>
                      </div>
                      <div className={`mt-1 h-2.5 w-2.5 rounded-full ${active ? "bg-amber-600" : "bg-slate-200"}`} />
                    </div>
                  </button>
                );
              })}
            </div>

            <div className="border-t border-[#ece2d6] px-4 py-4">
              <div className="text-xs font-semibold uppercase tracking-[0.28em] text-slate-400">Quick View</div>
              <div className="mt-3 grid gap-3">
                <div className="subtle-panel px-4 py-4">
                  <div className="text-xs uppercase tracking-[0.2em] text-slate-400">当前测试</div>
                  <div className="mt-2 font-mono text-sm text-slate-900">{selectedRoute ? `/${selectedRoute}` : "未选择路由"}</div>
                </div>
                <div className="subtle-panel px-4 py-4">
                  <div className="text-xs uppercase tracking-[0.2em] text-slate-400">Redis</div>
                  <div className="mt-2 text-sm text-slate-700">
                    {redisForm.enabled ? redisForm.addr || "已启用，未填写地址" : "当前未启用 Redis"}
                  </div>
                </div>
              </div>
            </div>
          </aside>

          <div className="space-y-5">
            <section className="card-panel flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
              <div>
                <div className="text-xs font-semibold uppercase tracking-[0.28em] text-clay">Workspace</div>
                <h2 className="mt-2 text-3xl font-semibold tracking-tight text-slate-950">{pageMeta.title}</h2>
                <p className="mt-3 max-w-3xl text-sm leading-7 text-slate-600">{pageMeta.description}</p>
              </div>
              <div className="grid gap-3 sm:grid-cols-2 lg:min-w-[360px]">
                <div className="metric-tile">
                  <div className="text-xs uppercase tracking-[0.2em] text-slate-400">Route</div>
                  <div className="mt-2 font-mono text-sm text-slate-900">{selectedRoutePath}</div>
                </div>
                <div className="metric-tile">
                  <div className="text-xs uppercase tracking-[0.2em] text-slate-400">Callback</div>
                  <div className="mt-2 font-mono text-sm text-slate-900">{selectedCallbackPath}</div>
                </div>
              </div>
            </section>

        {page === ADMIN_PAGES.config ? (
          <section className="grid gap-6 xl:grid-cols-[1.05fr_1fr]">
            <section className="card-panel space-y-5">
              <div className="flex items-center justify-between gap-4">
                <div>
                  <div className="text-sm uppercase tracking-[0.24em] text-clay">Config List</div>
                  <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">机器人配置列表</h2>
                  <p className="mt-2 text-sm leading-7 text-slate-600">
                    点击左侧任意机器人卡片，会自动在右侧展开对应配置进行编辑。
                  </p>
                </div>
                <button className="secondary-button" onClick={beginCreateConfig}>
                  新建配置
                </button>
              </div>

              <div className="overflow-hidden rounded-[18px] border border-[#e7ddd0] bg-[#fcfbf8]">
                {configs.length === 0 ? (
                  <div className="border border-dashed border-slate-300 bg-white/70 px-5 py-8 text-center text-sm text-slate-500">
                    还没有任何机器人配置。先在右侧创建一个路由。
                  </div>
                ) : (
                  <>
                    <div className="hidden grid-cols-[0.9fr_1fr_1.1fr_auto] gap-4 border-b border-[#ebe1d5] bg-[#f5efe6] px-5 py-3 text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500 lg:grid">
                      <div>路由</div>
                      <div>机器人</div>
                      <div>接收消息服务器</div>
                      <div className="text-right">操作</div>
                    </div>
                    {configs.map((config, index) => {
                      const isEditing = configMode === "edit" && editingRoute === config.route_suffix;
                      const isTesting = selectedRoute === config.route_suffix;
                      return (
                        <article
                          key={config.route_suffix}
                          role="button"
                          tabIndex={0}
                          onClick={() => beginEditConfig(config)}
                          onKeyDown={(event) => {
                            if (event.key === "Enter" || event.key === " ") {
                              event.preventDefault();
                              beginEditConfig(config);
                            }
                          }}
                          className={`cursor-pointer border-l-4 px-5 py-5 text-left transition ${
                            index < configs.length - 1 ? "border-b border-[#ece2d6]" : ""
                          } ${
                            isEditing
                              ? "border-l-slate-950 bg-slate-950 text-white"
                              : isTesting
                                ? "border-l-amber-500 bg-[#fff8ef] text-slate-900"
                                : "border-l-transparent bg-[#fcfbf8] text-slate-900 hover:bg-white"
                          }`}
                        >
                          <div className="grid gap-4 lg:grid-cols-[0.9fr_1fr_1.1fr_auto] lg:items-start">
                            <div>
                              <div className={`text-xs uppercase tracking-[0.22em] ${isEditing ? "text-amber-300" : "text-slate-400"}`}>
                                /{config.route_suffix}
                              </div>
                              <div className={`mt-3 flex flex-wrap gap-2 ${isEditing ? "text-white" : "text-slate-700"}`}>
                                {isEditing ? (
                                  <span className="rounded-full bg-amber-300/15 px-2.5 py-1 text-xs font-medium text-amber-300">当前正在编辑</span>
                                ) : null}
                                {isTesting ? (
                                  <span className={`rounded-full px-2.5 py-1 text-xs font-medium ${isEditing ? "bg-white/10 text-white" : "bg-slate-200/70 text-slate-700"}`}>
                                    当前测试目标
                                  </span>
                                ) : null}
                              </div>
                            </div>

                            <div>
                              <h3 className="text-lg font-semibold">{config.display_name || config.route_suffix}</h3>
                              <p className={`mt-2 text-sm ${isEditing ? "text-slate-300" : "text-slate-600"}`}>
                                企业ID: {config.wecom_cid || "-"}
                              </p>
                              <p className={`mt-1 text-sm ${isEditing ? "text-slate-300" : "text-slate-600"}`}>
                                AgentId: {config.wecom_aid || "-"}
                              </p>
                            </div>

                            <div>
                              <div className={`text-xs uppercase tracking-[0.2em] ${isEditing ? "text-slate-400" : "text-slate-400"}`}>callback</div>
                              <div className={`mt-2 break-all font-mono text-xs ${isEditing ? "text-slate-200" : "text-slate-700"}`}>
                                {callbackPath(config.route_suffix)}
                              </div>
                              <div className={`mt-3 text-xs uppercase tracking-[0.2em] ${isEditing ? "text-slate-400" : "text-slate-400"}`}>sendkey</div>
                              <div className={`mt-2 break-all font-mono text-xs ${isEditing ? "text-slate-200" : "text-slate-700"}`}>
                                {config.sendkey || "-"}
                              </div>
                            </div>

                            <div className="flex gap-3 lg:justify-end">
                              <button
                                className={`secondary-button ${isEditing ? "border-slate-700 bg-slate-900 text-white hover:border-slate-600 hover:bg-slate-800" : ""}`}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  beginEditConfig(config);
                                }}
                              >
                                编辑
                              </button>
                              <button
                                className={`secondary-button border-rose-200 text-rose-700 hover:border-rose-300 ${isEditing ? "border-rose-800 bg-transparent text-rose-200 hover:bg-rose-950/20" : ""}`}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  void handleDeleteConfig(config.route_suffix);
                                }}
                                disabled={busy.deleteConfig}
                              >
                                删除
                              </button>
                            </div>
                          </div>
                        </article>
                      );
                    })}
                  </>
                )}
              </div>
            </section>

            <section className="card-panel space-y-5">
              <div>
                <div className="text-sm uppercase tracking-[0.24em] text-clay">Config Editor</div>
                <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">
                  {configMode === "edit" ? `编辑配置 /${editingRoute}` : "新增机器人配置"}
                </h2>
                <p className="mt-2 text-sm leading-7 text-slate-600">
                  每个配置对应一个独立路由，比如 <code>/wecomchan</code>、<code>/another-bot</code>，callback 为{" "}
                  <code>/&lt;route&gt;/callback</code>。
                </p>
              </div>

              <form className="grid gap-4 md:grid-cols-2" onSubmit={handleSaveConfig}>
                <Field label="display_name" hint="列表展示名称">
                  <input
                    className="input-shell"
                    value={configForm.display_name}
                    onChange={(event) => updateConfigForm("display_name", event.target.value)}
                    placeholder="运营通知机器人"
                  />
                </Field>
                <Field label="route_suffix" hint="URL 路由后缀">
                  <input
                    className="input-shell"
                    value={configForm.route_suffix}
                    onChange={(event) => updateConfigForm("route_suffix", event.target.value)}
                    placeholder="wecomchan"
                  />
                </Field>
                <Field label="sendkey">
                  <input
                    className="input-shell"
                    value={configForm.sendkey}
                    onChange={(event) => updateConfigForm("sendkey", event.target.value)}
                  />
                </Field>
                <Field label="wecom_touid">
                  <input
                    className="input-shell"
                    value={configForm.wecom_touid}
                    onChange={(event) => updateConfigForm("wecom_touid", event.target.value)}
                    placeholder="@all"
                  />
                </Field>
                <Field label="企业ID">
                  <input
                    className="input-shell"
                    value={configForm.wecom_cid}
                    onChange={(event) => updateConfigForm("wecom_cid", event.target.value)}
                  />
                </Field>
                <Field label="AgentId">
                  <input
                    className="input-shell"
                    value={configForm.wecom_aid}
                    onChange={(event) => updateConfigForm("wecom_aid", event.target.value)}
                  />
                </Field>
                <div className="md:col-span-2">
                  <Field label="Secret">
                    <input
                      className="input-shell"
                      value={configForm.wecom_secret}
                      onChange={(event) => updateConfigForm("wecom_secret", event.target.value)}
                    />
                  </Field>
                </div>

                <div className="md:col-span-2 space-y-4 border border-slate-200 bg-stone-50 px-5 py-5">
                  <div>
                    <div className="text-sm font-semibold text-slate-900">接收消息服务器配置</div>
                    <p className="mt-2 text-sm leading-6 text-slate-600">
                      URL 填写的 URL 需要正确响应企业微信验证 URL 的请求。
                    </p>
                  </div>

                  <Field label="URL" hint="根据当前浏览器地址和路由后缀自动生成，不可编辑。">
                    <input className="input-shell bg-slate-100" value={callbackPreviewUrl} readOnly />
                  </Field>

                  <div className="grid gap-4 md:grid-cols-2">
                    <Field label="Token">
                      <input
                        className="input-shell"
                        value={configForm.callback_token}
                        onChange={(event) => updateConfigForm("callback_token", event.target.value)}
                      />
                    </Field>
                    <Field label="EncodingAESKey">
                      <input
                        className="input-shell"
                        value={configForm.callback_aes_key}
                        onChange={(event) => updateConfigForm("callback_aes_key", event.target.value)}
                      />
                    </Field>
                  </div>
                </div>

                <div className="md:col-span-2 flex flex-wrap gap-3">
                  <button type="submit" className="primary-button" disabled={busy.saveConfig}>
                    {busy.saveConfig ? "保存中..." : configMode === "edit" ? "保存配置" : "创建配置"}
                  </button>
                  <button type="button" className="secondary-button" onClick={beginCreateConfig}>
                    新建空白配置
                  </button>
                </div>
              </form>

              {configFeedback ? (
                <div className="rounded-lg border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700">{configFeedback}</div>
              ) : null}

              <CodeBlock title="当前编辑路由">
                {editingConfig
                  ? `POST ${routePath(editingConfig.route_suffix)}\nGET ${callbackPath(editingConfig.route_suffix)}`
                  : "当前是新建模式，保存后会生成独立路由和 callback 地址。"}
              </CodeBlock>
            </section>
          </section>
        ) : null}

        {page === ADMIN_PAGES.redis ? (
          <section className="card-panel space-y-5">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <div className="text-sm uppercase tracking-[0.24em] text-clay">Global Redis</div>
                <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">统一 Redis 配置</h2>
                <p className="mt-2 max-w-3xl text-sm leading-7 text-slate-600">
                  所有机器人共用同一个 Redis 服务，token key 会按路由后缀、企业 ID 和 AgentId 自动区分。
                </p>
              </div>
              <div className="rounded-lg border border-slate-200 bg-white/85 px-4 py-3 text-sm text-slate-700">
                {redisForm.enabled ? `已启用: ${redisForm.addr || "未填写地址"}` : "当前未启用 Redis 缓存"}
              </div>
            </div>

            <form className="grid gap-4 md:grid-cols-2" onSubmit={handleSaveRedis}>
              <div className="md:col-span-2">
                <label className="flex items-center gap-3 rounded-lg border border-slate-200 bg-white/85 px-4 py-3 text-sm text-slate-700">
                  <input
                    type="checkbox"
                    checked={redisForm.enabled}
                    onChange={(event) => updateRedisForm("enabled", event.target.checked)}
                  />
                  启用全局 Redis token 缓存
                </label>
              </div>
              <Field label="redis.addr" hint="所有机器人共享同一个 Redis 地址。">
                <input
                  className="input-shell"
                  value={redisForm.addr}
                  onChange={(event) => updateRedisForm("addr", event.target.value)}
                  placeholder="redis:6379"
                />
              </Field>
              <Field label="redis.password">
                <input
                  className="input-shell"
                  value={redisForm.password}
                  onChange={(event) => updateRedisForm("password", event.target.value)}
                />
              </Field>
              <div className="md:col-span-2 flex flex-wrap gap-3">
                <button type="submit" className="primary-button" disabled={busy.saveRedis}>
                  {busy.saveRedis ? "保存中..." : "保存 Redis 配置"}
                </button>
              </div>
            </form>

            {redisFeedback ? (
              <div className="rounded-lg border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700">{redisFeedback}</div>
            ) : null}

            <CodeBlock title="说明">
              {`1. Redis 配置是全局唯一，不再跟随单个机器人重复保存。\n2. 备份 bot-config.json 时，Redis 和机器人配置会一起备份。\n3. 如果关闭 Redis，服务会直接回退到实时获取 token。`}
            </CodeBlock>
          </section>
        ) : null}

        {page === ADMIN_PAGES.tools ? (
          <>
            <section className="card-panel space-y-5">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <div className="text-sm uppercase tracking-[0.24em] text-clay">Selected Route</div>
                  <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">接口调试使用的配置</h2>
                </div>
                <div className="rounded-lg border border-slate-200 bg-white/85 px-4 py-3 text-sm text-slate-700">
                  {selectedConfig ? `当前路由: /${selectedConfig.route_suffix}` : "未选择配置"}
                </div>
              </div>
              <div className="grid gap-4 lg:grid-cols-[1.2fr_1fr_1fr_1fr]">
                <Field label="route_suffix" hint="切换当前要调试的机器人路由。">
                  <select
                    className="input-shell"
                    value={selectedRoute}
                    onChange={(event) => setSelectedRoute(event.target.value)}
                    disabled={configs.length === 0}
                  >
                    {configs.length === 0 ? <option value="">暂无配置</option> : null}
                    {configs.map((config) => (
                      <option key={config.route_suffix} value={config.route_suffix}>
                        /{config.route_suffix} · {config.display_name || config.route_suffix}
                      </option>
                    ))}
                  </select>
                </Field>
                <div className="rounded-lg border border-slate-200 bg-white/85 px-4 py-4">
                  <div className="text-xs uppercase tracking-[0.2em] text-slate-400">Send URL</div>
                  <div className="mt-2 font-mono text-sm text-slate-900">{selectedRoutePath}</div>
                </div>
                <div className="rounded-lg border border-slate-200 bg-white/85 px-4 py-4">
                  <div className="text-xs uppercase tracking-[0.2em] text-slate-400">Callback URL</div>
                  <div className="mt-2 font-mono text-sm text-slate-900">{selectedCallbackPath}</div>
                </div>
                <div className="rounded-lg border border-slate-200 bg-white/85 px-4 py-4">
                  <div className="text-xs uppercase tracking-[0.2em] text-slate-400">Config Store</div>
                  <div className="mt-2 break-all font-mono text-sm text-slate-900">{session.configStorePath || "./data/bot-config.json"}</div>
                </div>
              </div>
            </section>

            <ToolCard
              badge="GET"
              title={getTextDoc.title}
              description={getTextDoc.description}
              endpointMethod={getTextDoc.method}
              endpointPath={getTextEndpointPath}
              template={getTemplate}
              params={getTextDoc.params}
              execution={results.getText}
            >
              <Field label="route_suffix" hint="当前选中的机器人路由">
                <input className="input-shell" value={selectedRoute} readOnly />
              </Field>
              <Field label="消息内容">
                <textarea
                  value={forms.getTextMessage}
                  onChange={(event) => updateForm("getTextMessage", event.target.value)}
                  rows={5}
                  className="input-shell min-h-32"
                />
              </Field>
              <button className="primary-button" onClick={executeGetText} disabled={busy.getText || !selectedRoute}>
                {busy.getText ? "执行中..." : "执行 GET 文本发送"}
              </button>
            </ToolCard>

            <ToolCard
              badge="POST"
              title={postDoc.title}
              description={postDoc.description}
              endpointMethod={postDoc.method}
              endpointPath={postEndpointPath}
              template={postTemplate}
              params={postDoc.params}
              execution={results.postMessage}
            >
              <Field label="route_suffix">
                <input className="input-shell" value={selectedRoute} readOnly />
              </Field>
              <Field label="消息类型">
                <select
                  value={forms.postMessageType}
                  onChange={(event) => updateForm("postMessageType", event.target.value)}
                  className="input-shell"
                >
                  <option value="text">text</option>
                  <option value="markdown">markdown</option>
                </select>
              </Field>
              <Field label="消息体">
                <textarea
                  value={forms.postMessageBody}
                  onChange={(event) => updateForm("postMessageBody", event.target.value)}
                  rows={8}
                  className="input-shell min-h-44"
                />
              </Field>
              <button className="primary-button" onClick={executePostMessage} disabled={busy.postMessage || !selectedRoute}>
                {busy.postMessage ? "执行中..." : "执行 POST 文本发送"}
              </button>
            </ToolCard>

            <ToolCard
              badge="IMAGE"
              title={imageDoc.title}
              description={imageDoc.description}
              endpointMethod={imageDoc.method}
              endpointPath={imageEndpointPath}
              template={imageTemplate}
              params={imageDoc.params}
              execution={results.image}
            >
              <Field label="route_suffix">
                <input className="input-shell" value={selectedRoute} readOnly />
              </Field>
              <Field label="选择图片" hint="原图大于 2MB 时，会依次尝试高质量、中等质量、低质量 JPEG 压缩。">
                <input
                  type="file"
                  accept="image/*"
                  className="input-shell file:mr-4 file:rounded-md file:border-0 file:bg-slate-950 file:px-4 file:py-2 file:text-sm file:font-medium file:text-white"
                  onChange={(event) => updateForm("imageFile", event.target.files?.[0] || null)}
                />
              </Field>
              {forms.imageFile ? (
                <div className="rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800">
                  当前文件：{forms.imageFile.name} ({formatBytes(forms.imageFile.size)})
                </div>
              ) : null}
              <button className="primary-button" onClick={executeImage} disabled={busy.image || !selectedRoute}>
                {busy.image ? "执行中..." : "执行图片发送"}
              </button>
            </ToolCard>

            <ToolCard
              badge="TEXT+IMAGE"
              title={textImageDoc.title}
              description={textImageDoc.description}
              endpointMethod={textImageDoc.method}
              endpointPath={textImageEndpointPath}
              template={textImageTemplate}
              params={textImageDoc.params}
              execution={results.textImage}
            >
              <Field label="route_suffix">
                <input className="input-shell" value={selectedRoute} readOnly />
              </Field>
              <Field label="文本内容">
                <textarea
                  value={forms.textImageMessage}
                  onChange={(event) => updateForm("textImageMessage", event.target.value)}
                  rows={5}
                  className="input-shell min-h-32"
                />
              </Field>
              <Field label="图片文件" hint="执行时前端会先把图片压缩到 2MB 内，再转为 base64 调用当前路由。">
                <input
                  type="file"
                  accept="image/*"
                  className="input-shell file:mr-4 file:rounded-md file:border-0 file:bg-slate-950 file:px-4 file:py-2 file:text-sm file:font-medium file:text-white"
                  onChange={(event) => updateForm("textImageFile", event.target.files?.[0] || null)}
                />
              </Field>
              {forms.textImageFile ? (
                <div className="rounded-lg border border-sky-200 bg-sky-50 px-4 py-3 text-sm text-sky-800">
                  待编码文件：{forms.textImageFile.name} ({formatBytes(forms.textImageFile.size)})
                </div>
              ) : null}
              <button className="primary-button" onClick={executeTextImage} disabled={busy.textImage || !selectedRoute}>
                {busy.textImage ? "编码并发送中..." : "执行图文发送"}
              </button>
            </ToolCard>

            <section className="card-panel space-y-5">
              <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                <div>
                  <div className="inline-flex rounded-md bg-emerald-100 px-3 py-1 text-xs font-semibold uppercase tracking-[0.22em] text-emerald-700">
                    Verification
                  </div>
                  <h2 className="mt-3 text-2xl font-semibold tracking-tight text-slate-950">服务器消息校验接口说明</h2>
                  <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-600">
                    这里会根据当前选中的路由配置，展示对应的 callback 地址和参数说明。
                  </p>
                </div>
              </div>

              <div className="grid gap-5 xl:grid-cols-2">
                {(docs.verification_routes || FALLBACK_DOCS.verification_routes).map((item) => (
                  <article key={`${item.method}-${item.path}-${item.title}`} className="rounded-lg border border-slate-200 bg-white/85 p-5">
                    <div className="flex flex-wrap items-center gap-3">
                      <span className="inline-flex rounded-md bg-emerald-100 px-3 py-1 text-xs font-semibold uppercase tracking-[0.22em] text-emerald-700">
                        {item.method}
                      </span>
                      <span className="font-mono text-xs text-slate-700">{formatDocUrlForBrowser(item.path, browserOrigin)}</span>
                    </div>
                    <h3 className="mt-3 text-xl font-semibold text-slate-950">{item.title}</h3>
                    <p className="mt-2 text-sm leading-6 text-slate-600">{item.description}</p>
                    <div className="mt-4">
                      <ParamTable params={item.params} />
                    </div>
                  </article>
                ))}
              </div>
            </section>
          </>
        ) : null}

        {page === ADMIN_PAGES.logs ? (
          <>
            <section className="card-panel space-y-5">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <div className="text-sm uppercase tracking-[0.24em] text-clay">Message Logs</div>
                  <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">最近发送记录</h2>
                  <p className="mt-2 max-w-3xl text-sm leading-7 text-slate-600">
                    默认展示最近 7 天的发送记录。文本和图片会分别记录，图文双发会显示为两条发送消息。
                  </p>
                </div>
                <div className="rounded-lg border border-slate-200 bg-white/85 px-4 py-3 text-sm text-slate-700">
                  当前筛选：{logQuery.routeSuffix ? `/${logQuery.routeSuffix}` : "全部路由"}
                </div>
              </div>

              <div className="grid gap-4 lg:grid-cols-[0.8fr_0.8fr_1fr_auto]">
                <Field label="最近天数">
                  <input
                    type="number"
                    min="1"
                    className="input-shell"
                    value={logQuery.days}
                    onChange={(event) => updateLogQuery("days", event.target.value)}
                  />
                </Field>
                <Field label="最多返回">
                  <input
                    type="number"
                    min="1"
                    className="input-shell"
                    value={logQuery.limit}
                    onChange={(event) => updateLogQuery("limit", event.target.value)}
                  />
                </Field>
                <Field label="route_suffix">
                  <select
                    className="input-shell"
                    value={logQuery.routeSuffix}
                    onChange={(event) => updateLogQuery("routeSuffix", event.target.value)}
                  >
                    <option value="">全部路由</option>
                    {configs.map((config) => (
                      <option key={config.route_suffix} value={config.route_suffix}>
                        /{config.route_suffix} · {config.display_name || config.route_suffix}
                      </option>
                    ))}
                  </select>
                </Field>
                <div className="flex items-end">
                  <button className="primary-button w-full lg:w-auto" onClick={loadMessageLogs} disabled={busy.loadLogs}>
                    {busy.loadLogs ? "刷新中..." : "刷新日志"}
                  </button>
                </div>
              </div>

              {logFeedback ? (
                <div className="rounded-lg border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700">{logFeedback}</div>
              ) : null}
              {logError ? (
                <div className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{logError}</div>
              ) : null}
            </section>

            <section className="space-y-4">
              {messageLogs.length === 0 ? (
                <section className="card-panel text-sm text-slate-500">最近 {logQuery.days} 天还没有匹配的发送记录。</section>
              ) : (
                <section className="overflow-hidden rounded-[18px] border border-[#e7ddd0] bg-[#fcfbf8]">
                  <div className="hidden grid-cols-[1fr_1.4fr_0.8fr_0.9fr] gap-4 border-b border-[#ebe1d5] bg-[#f5efe6] px-5 py-3 text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500 lg:grid">
                    <div>路由 / 类型</div>
                    <div>内容预览</div>
                    <div>文件信息</div>
                    <div>发送时间</div>
                  </div>
                  {messageLogs.map((item, index) => (
                    <article
                      key={`${item.timestamp}-${item.route_suffix}-${item.msg_type}-${index}`}
                      className={`grid gap-4 px-5 py-5 lg:grid-cols-[1fr_1.4fr_0.8fr_0.9fr] ${
                        index < messageLogs.length - 1 ? "border-b border-[#ece2d6]" : ""
                      }`}
                    >
                      <div>
                        <div className="inline-flex rounded-full border border-slate-200 bg-slate-100 px-2.5 py-1 text-xs font-semibold uppercase tracking-[0.2em] text-slate-700">
                          {item.msg_type || "unknown"}
                        </div>
                        <h3 className="mt-3 text-lg font-semibold tracking-tight text-slate-950">
                          /{item.route_suffix || "-"}
                        </h3>
                        <p className="mt-2 text-sm text-slate-600">{item.display_name || item.route_suffix || "未命名机器人"}</p>
                      </div>

                      <div className="subtle-panel px-4 py-4">
                        <div className="text-xs uppercase tracking-[0.2em] text-slate-400">内容预览</div>
                        <div className="mt-3 whitespace-pre-wrap break-words text-sm leading-7 text-slate-700">
                          {item.content_preview || "图片消息无文本预览。"}
                        </div>
                      </div>

                      <div className="space-y-3 text-sm text-slate-700">
                        <div>
                          <div className="text-xs uppercase tracking-[0.2em] text-slate-400">文件名</div>
                          <div className="mt-2 break-all font-mono text-xs text-slate-900">{item.filename || "-"}</div>
                        </div>
                        <div>
                          <div className="text-xs uppercase tracking-[0.2em] text-slate-400">大小</div>
                          <div className="mt-2 font-mono text-xs text-slate-900">{item.size_bytes ? formatBytes(item.size_bytes) : "-"}</div>
                        </div>
                      </div>

                      <div>
                        <div className="text-xs uppercase tracking-[0.2em] text-slate-400">发送时间</div>
                        <p className="mt-3 text-sm leading-7 text-slate-700">{formatDateTime(item.timestamp)}</p>
                      </div>
                    </article>
                  ))}
                </section>
              )}
            </section>
          </>
        ) : null}
          </div>
        </div>
      </div>
    </main>
  );
}
