"use strict";

const $ = (id) => document.getElementById(id);
const storageKey = "hanxu-assistant-demo-v1";

function loadSaved() {
  try {
    const value = JSON.parse(localStorage.getItem(storageKey) || "null");
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}

const saved = loadSaved();
let tasks = Array.isArray(saved.tasks)
  ? saved.tasks.filter((item) => item && typeof item.text === "string").slice(0, 30)
      .map((item) => ({ id: String(item.id), text: item.text.slice(0, 80), done: item.done === true }))
  : [
      { id: "sample-1", text: "整理今天最重要的三件事", done: false },
      { id: "sample-2", text: "留一段不被打扰的专注时间", done: false },
      { id: "sample-3", text: "记录一个新想法", done: false },
    ];
let notes = typeof saved.notes === "string" ? saved.notes.slice(0, 5000) : "";
let theme = saved.theme === "dark" || saved.theme === "light"
  ? saved.theme
  : (window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");

function save() {
  try {
    localStorage.setItem(storageKey, JSON.stringify({ tasks, notes, theme }));
    return true;
  } catch {
    return false;
  }
}

function renderTasks() {
  const list = $("task-list");
  list.replaceChildren();
  for (const task of tasks) {
    const item = document.createElement("li");
    item.className = "task-item" + (task.done ? " is-done" : "");

    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.checked = task.done;
    checkbox.setAttribute("aria-label", `完成：${task.text}`);
    checkbox.addEventListener("change", () => {
      task.done = checkbox.checked;
      save();
      renderTasks();
    });

    const label = document.createElement("span");
    label.textContent = task.text;

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "delete-button";
    remove.textContent = "删除";
    remove.setAttribute("aria-label", `删除：${task.text}`);
    remove.addEventListener("click", () => {
      tasks = tasks.filter((entry) => entry.id !== task.id);
      save();
      renderTasks();
    });

    item.append(checkbox, label, remove);
    list.append(item);
  }
  if (tasks.length === 0) {
    const empty = document.createElement("li");
    empty.className = "empty-list";
    empty.textContent = "清单还是空的，先添加一件小事吧。";
    list.append(empty);
  }
  const done = tasks.filter((task) => task.done).length;
  const remaining = tasks.length - done;
  $("task-count").textContent = `${remaining} 项待完成`;
  $("hero-progress").textContent = `${done} / ${tasks.length}`;
  $("hero-progress-bar").style.width = `${tasks.length ? (done / tasks.length) * 100 : 0}%`;
  $("clear-completed").disabled = done === 0;
}

$("task-form").addEventListener("submit", (event) => {
  event.preventDefault();
  const input = $("task-input");
  const text = input.value.trim();
  if (!text || tasks.length >= 30) {
    if (tasks.length >= 30) input.setCustomValidity("演示版最多保存 30 项待办");
    input.reportValidity();
    return;
  }
  input.setCustomValidity("");
  tasks.unshift({ id: `${Date.now()}-${Math.random().toString(36).slice(2)}`, text, done: false });
  input.value = "";
  save();
  renderTasks();
  input.focus();
});
$("task-input").addEventListener("input", (event) => event.currentTarget.setCustomValidity(""));
$("clear-completed").addEventListener("click", () => {
  tasks = tasks.filter((task) => !task.done);
  save();
  renderTasks();
});

const notesInput = $("notes-input");
notesInput.value = notes;
function updateNotesCount() { $("notes-count").textContent = `${notesInput.value.length} / 5000`; }
updateNotesCount();
let saveNotesTimer;
notesInput.addEventListener("input", () => {
  notes = notesInput.value;
  updateNotesCount();
  $("notes-status").textContent = "正在保存…";
  clearTimeout(saveNotesTimer);
  saveNotesTimer = setTimeout(() => {
    $("notes-status").textContent = save() ? "已保存在当前浏览器" : "无法保存，请检查浏览器存储权限";
  }, 350);
});
window.addEventListener("pagehide", () => {
  if (saveNotesTimer) save();
});

function applyTheme() {
  document.documentElement.dataset.theme = theme;
  $("theme-toggle").textContent = theme === "dark" ? "浅色模式" : "深色模式";
  $("theme-toggle").setAttribute("aria-label", `切换${theme === "dark" ? "浅色" : "深色"}模式`);
}
$("theme-toggle").addEventListener("click", () => {
  theme = theme === "dark" ? "light" : "dark";
  applyTheme();
  save();
});
applyTheme();

let duration = 25 * 60;
let remainingSeconds = duration;
let endAt = null;
let timerInterval = null;

function renderTimer() {
  const minutes = Math.floor(remainingSeconds / 60).toString().padStart(2, "0");
  const seconds = (remainingSeconds % 60).toString().padStart(2, "0");
  $("timer-display").textContent = `${minutes}:${seconds}`;
  document.title = endAt ? `${minutes}:${seconds} · 个人工具助手` : "个人工具助手 · Hanxu Song";
  $("timer-toggle").textContent = endAt ? "暂停" : remainingSeconds < duration ? "继续" : duration === 300 ? "开始休息" : "开始专注";
}

function stopTimer() {
  endAt = null;
  if (timerInterval) clearInterval(timerInterval);
  timerInterval = null;
}

function tick() {
  if (!endAt) return;
  remainingSeconds = Math.max(0, Math.ceil((endAt - Date.now()) / 1000));
  renderTimer();
  if (remainingSeconds === 0) {
    stopTimer();
    $("timer-status").textContent = duration === 300 ? "休息结束，准备开始下一段专注。" : "这一段专注完成了，辛苦了。";
    renderTimer();
  }
}

$("timer-toggle").addEventListener("click", () => {
  if (endAt) {
    tick();
    stopTimer();
    $("timer-status").textContent = "已暂停，随时可以继续。";
  } else {
    if (remainingSeconds === 0) remainingSeconds = duration;
    endAt = Date.now() + remainingSeconds * 1000;
    timerInterval = setInterval(tick, 250);
    $("timer-status").textContent = duration === 300 ? "休息一下，稍后再出发。" : "现在只做眼前这一件事。";
  }
  renderTimer();
});
$("timer-reset").addEventListener("click", () => {
  stopTimer();
  remainingSeconds = duration;
  $("timer-status").textContent = "准备好就开始。";
  renderTimer();
});
document.querySelectorAll(".preset").forEach((button) => {
  button.addEventListener("click", () => {
    stopTimer();
    duration = Number(button.dataset.minutes) * 60;
    remainingSeconds = duration;
    document.querySelectorAll(".preset").forEach((preset) => {
      const active = preset === button;
      preset.classList.toggle("is-active", active);
      preset.setAttribute("aria-pressed", String(active));
    });
    $("timer-status").textContent = "准备好就开始。";
    renderTimer();
  });
});
renderTimer();

function updateClock() {
  const now = new Date();
  $("today-date").textContent = new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "long", day: "numeric", weekday: "long" }).format(now);
  $("today-clock").textContent = new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false }).format(now);
  $("footer-year").textContent = String(now.getFullYear());
}
updateClock();
setInterval(updateClock, 30_000);
renderTasks();
