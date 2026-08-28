import { defineComponent as k, openBlock as t, createElementBlock as n, normalizeClass as z, createCommentVNode as m, createElementVNode as s, renderSlot as _, toDisplayString as d, createBlock as C, withCtx as I, createTextVNode as K, createVNode as T, computed as L, Fragment as H, renderList as S, ref as E, onMounted as O, nextTick as P, watch as q, normalizeStyle as V, render as Z, inject as j, provide as ee, useId as U, onBeforeUnmount as te, Teleport as ae, withModifiers as ne, withDirectives as W, vModelCheckbox as le, unref as M, vShow as ie, createSlots as se, onUnmounted as oe, onErrorCaptured as de } from "vue";
const re = ["type", "disabled", "aria-busy", "aria-disabled", "aria-description", "title"], ce = {
  key: 0,
  class: "hnb-button__spinner",
  "aria-hidden": "true"
}, ue = /* @__PURE__ */ k({
  __name: "HNBButton",
  props: {
    variant: { default: "secondary" },
    size: { default: "medium" },
    disabled: { type: Boolean, default: !1 },
    loading: { type: Boolean, default: !1 },
    type: { default: "button" },
    disabledReason: {}
  },
  setup(e) {
    return (o, a) => (t(), n("button", {
      class: z(["hnb-button", [`hnb-button--${e.variant}`, `hnb-button--${e.size}`, { "hnb-button--loading": e.loading }]]),
      type: e.type,
      disabled: e.disabled || e.loading,
      "aria-busy": e.loading,
      "aria-disabled": e.disabled || e.loading,
      "aria-description": e.disabled && e.disabledReason ? e.disabledReason : void 0,
      title: e.disabled && e.disabledReason ? e.disabledReason : void 0
    }, [
      e.loading ? (t(), n("span", ce)) : m("", !0),
      s("span", {
        class: z({ "hnb-button__content--hidden": e.loading })
      }, [
        _(o.$slots, "default", {}, void 0, !0)
      ], 2)
    ], 10, re));
  }
}), $ = (e, o) => {
  const a = e.__vccOpts || e;
  for (const [i, l] of o)
    a[i] = l;
  return a;
}, D = /* @__PURE__ */ $(ue, [["__scopeId", "data-v-8cd6d6ac"]]), be = { class: "empty-state" }, fe = { class: "empty-title" }, me = {
  key: 0,
  class: "empty-description"
}, he = /* @__PURE__ */ k({
  __name: "EmptyState",
  props: {
    title: {},
    description: {},
    actionText: {}
  },
  emits: ["action"],
  setup(e, { emit: o }) {
    const a = o;
    return (i, l) => (t(), n("div", be, [
      l[1] || (l[1] = s("div", {
        class: "empty-icon",
        "aria-hidden": "true"
      }, "∅", -1)),
      s("div", fe, d(e.title), 1),
      e.description ? (t(), n("div", me, d(e.description), 1)) : m("", !0),
      e.actionText ? (t(), C(D, {
        key: 1,
        class: "empty-action",
        variant: "primary",
        onClick: l[0] || (l[0] = (c) => a("action"))
      }, {
        default: I(() => [
          K(d(e.actionText), 1)
        ]),
        _: 1
      })) : m("", !0)
    ]));
  }
}), Y = /* @__PURE__ */ $(he, [["__scopeId", "data-v-19451507"]]), ye = {
  class: "error-state",
  role: "alert"
}, ve = { class: "error-title" }, ge = {
  key: 0,
  class: "error-description"
}, $e = {
  key: 1,
  class: "error-code"
}, ke = /* @__PURE__ */ k({
  __name: "ErrorState",
  props: {
    title: { default: "加载失败" },
    description: {},
    retryText: { default: "重试" },
    retryLoading: { type: Boolean, default: !1 },
    code: {}
  },
  emits: ["retry"],
  setup(e, { emit: o }) {
    const a = o;
    return (i, l) => (t(), n("div", ye, [
      l[1] || (l[1] = s("div", {
        class: "error-icon",
        "aria-hidden": "true"
      }, "!", -1)),
      s("div", ve, d(e.title), 1),
      e.description ? (t(), n("div", ge, d(e.description), 1)) : m("", !0),
      e.code ? (t(), n("div", $e, d(e.code), 1)) : m("", !0),
      T(D, {
        class: "error-retry",
        loading: e.retryLoading,
        onClick: l[0] || (l[0] = (c) => a("retry"))
      }, {
        default: I(() => [
          K(d(e.retryText), 1)
        ]),
        _: 1
      }, 8, ["loading"])
    ]));
  }
}), G = /* @__PURE__ */ $(ke, [["__scopeId", "data-v-addeca22"]]), pe = ["aria-live"], Be = /* @__PURE__ */ k({
  __name: "HNBLiveRegion",
  props: {
    message: {},
    politeness: { default: "polite" }
  },
  setup(e) {
    return (o, a) => (t(), n("div", {
      class: "hnb-live-region",
      "aria-live": e.politeness,
      "aria-atomic": "true"
    }, d(e.message), 9, pe));
  }
}), J = /* @__PURE__ */ $(Be, [["__scopeId", "data-v-da0e18d4"]]), we = ["aria-label"], _e = {
  class: "hnb-pagination__status",
  "aria-current": "page"
}, xe = { class: "hnb-pagination__size" }, He = ["value"], Ie = ["value"], Ne = /* @__PURE__ */ k({
  __name: "HNBPagination",
  props: {
    page: {},
    pageSize: {},
    total: {},
    pageSizes: { default: () => [10, 20, 50] },
    ariaLabel: { default: "Pagination" },
    previousLabel: { default: "Previous page" },
    nextLabel: { default: "Next page" },
    pageSizeLabel: { default: "Items per page" },
    statusText: { default: "" },
    previousDisabledReason: { default: "Already on the first page" },
    nextDisabledReason: { default: "Already on the last page" }
  },
  emits: ["update:page", "update:pageSize"],
  setup(e, { emit: o }) {
    const a = e, i = o, l = L(() => Math.max(1, Math.ceil(a.total / a.pageSize))), c = L(() => a.statusText ? a.statusText : `Page ${a.page} of ${l.value}, ${a.total} items`);
    function g(f) {
      i("update:pageSize", Number(f.target.value));
    }
    return (f, r) => (t(), n("nav", {
      class: "hnb-pagination",
      "aria-label": e.ariaLabel
    }, [
      T(D, {
        size: "small",
        disabled: e.page <= 1,
        "disabled-reason": e.previousDisabledReason,
        "aria-label": e.previousLabel,
        onClick: r[0] || (r[0] = (p) => i("update:page", e.page - 1))
      }, {
        default: I(() => [...r[2] || (r[2] = [
          K("‹", -1)
        ])]),
        _: 1
      }, 8, ["disabled", "disabled-reason", "aria-label"]),
      s("span", _e, d(c.value), 1),
      T(D, {
        size: "small",
        disabled: e.page >= l.value,
        "disabled-reason": e.nextDisabledReason,
        "aria-label": e.nextLabel,
        onClick: r[1] || (r[1] = (p) => i("update:page", e.page + 1))
      }, {
        default: I(() => [...r[3] || (r[3] = [
          K("›", -1)
        ])]),
        _: 1
      }, 8, ["disabled", "disabled-reason", "aria-label"]),
      s("label", xe, [
        s("span", null, d(e.pageSizeLabel), 1),
        s("select", {
          value: e.pageSize,
          onChange: g
        }, [
          (t(!0), n(H, null, S(e.pageSizes, (p) => (t(), n("option", {
            key: p,
            value: p
          }, d(p), 9, Ie))), 128))
        ], 40, He)
      ]),
      T(J, { message: c.value }, null, 8, ["message"])
    ], 8, we));
  }
}), Le = /* @__PURE__ */ $(Ne, [["__scopeId", "data-v-538c1b4b"]]), Se = ["aria-label"], Re = { class: "hnb-table" }, Ce = {
  key: 0,
  class: "hnb-table__check"
}, Ke = ["checked", "indeterminate"], Te = { key: 0 }, Ve = ["colspan"], De = { key: 1 }, Ee = ["colspan"], ze = { key: 2 }, Me = ["colspan"], Ae = {
  key: 0,
  class: "hnb-table__check"
}, Pe = ["checked", "onChange"], qe = ["data-cell-key"], Fe = /* @__PURE__ */ k({
  __name: "HNBTable",
  props: {
    columns: {},
    data: {},
    loading: { type: Boolean, default: !1 },
    rowKey: {},
    pagination: {},
    selectable: { type: Boolean, default: !1 },
    checkedRowKeys: { default: () => [] },
    ariaLabel: { default: "Data table" },
    minWidth: { default: "640px" },
    emptyTitle: { default: "暂无数据" },
    error: {},
    errorRetryText: { default: "重试" },
    errorRetryLoading: { type: Boolean, default: !1 }
  },
  emits: ["update:page", "update:pageSize", "update:checkedRowKeys", "errorRetry"],
  setup(e, { emit: o }) {
    const a = e, i = o, l = E(null);
    function c(v, h, y, R) {
      if (!y.render) return;
      v.innerHTML = "";
      const b = y.render(h, R);
      b != null && (typeof b == "string" || typeof b == "number" ? v.textContent = String(b) : Z(b, v));
    }
    function g() {
      const v = l.value;
      if (!v) return;
      v.querySelectorAll("[data-cell-key]").forEach((y) => {
        const b = (y.getAttribute("data-cell-key") || "").split("::");
        if (b.length !== 2) return;
        const [x, B] = b, w = parseInt(x, 10), N = a.data[w], A = a.columns.find((X) => X.key === B);
        N && A && c(y, N, A, w);
      });
    }
    O(() => {
      P(() => g());
    }), q(() => a.data, () => {
      P(() => g());
    }, { deep: !0 }), q(() => a.columns, () => {
      P(() => g());
    }, { deep: !0 });
    function f(v) {
      if (!a.rowKey) return !1;
      if (typeof a.rowKey == "string")
        return a.checkedRowKeys.includes(v[a.rowKey]);
      const h = a.rowKey(v);
      return a.checkedRowKeys.includes(h);
    }
    function r(v) {
      if (!a.rowKey) return;
      let h;
      typeof a.rowKey == "string" ? h = v[a.rowKey] : h = a.rowKey(v);
      const y = f(v) ? a.checkedRowKeys.filter((R) => R !== h) : [...a.checkedRowKeys, h];
      i("update:checkedRowKeys", y);
    }
    function p() {
      if (u.value)
        i("update:checkedRowKeys", []);
      else {
        const v = a.data.map((h) => typeof a.rowKey == "string" ? h[a.rowKey] : a.rowKey(h));
        i("update:checkedRowKeys", v);
      }
    }
    const u = L(() => a.data.length ? a.data.every((v) => f(v)) : !1);
    return (v, h) => (t(), n("div", {
      ref_key: "tableRef",
      ref: l,
      class: "hnb-table-wrapper",
      style: V({ minWidth: e.minWidth }),
      role: "region",
      "aria-label": e.ariaLabel,
      tabindex: "0"
    }, [
      s("table", Re, [
        s("thead", null, [
          s("tr", null, [
            e.selectable ? (t(), n("th", Ce, [
              s("input", {
                type: "checkbox",
                checked: u.value,
                indeterminate: !u.value && e.checkedRowKeys.length > 0,
                onChange: p
              }, null, 40, Ke)
            ])) : m("", !0),
            (t(!0), n(H, null, S(e.columns, (y) => (t(), n("th", {
              key: y.key,
              style: V(y.width ? { width: y.width } : void 0)
            }, d(y.title), 5))), 128))
          ])
        ]),
        s("tbody", null, [
          e.loading ? (t(), n("tr", Te, [
            s("td", {
              colspan: e.columns.length + (e.selectable ? 1 : 0),
              class: "hnb-table__loading"
            }, [...h[3] || (h[3] = [
              s("div", { class: "hnb-table__spinner" }, null, -1)
            ])], 8, Ve)
          ])) : e.error ? (t(), n("tr", De, [
            s("td", {
              colspan: e.columns.length + (e.selectable ? 1 : 0),
              class: "hnb-table__state"
            }, [
              T(G, {
                title: e.error,
                "retry-text": e.errorRetryText,
                "retry-loading": e.errorRetryLoading,
                onRetry: h[0] || (h[0] = (y) => i("errorRetry"))
              }, null, 8, ["title", "retry-text", "retry-loading"])
            ], 8, Ee)
          ])) : e.data.length ? m("", !0) : (t(), n("tr", ze, [
            s("td", {
              colspan: e.columns.length + (e.selectable ? 1 : 0),
              class: "hnb-table__state"
            }, [
              T(Y, { title: e.emptyTitle }, null, 8, ["title"])
            ], 8, Me)
          ])),
          (t(!0), n(H, null, S(e.data, (y, R) => (t(), n("tr", {
            key: e.rowKey ? typeof e.rowKey == "string" ? y[e.rowKey] : e.rowKey(y) : R,
            class: z({ "hnb-table__row--checked": f(y) })
          }, [
            e.selectable ? (t(), n("td", Ae, [
              s("input", {
                type: "checkbox",
                checked: f(y),
                onChange: (b) => r(y)
              }, null, 40, Pe)
            ])) : m("", !0),
            (t(!0), n(H, null, S(e.columns, (b) => (t(), n("td", {
              key: b.key
            }, [
              s("div", {
                "data-cell-key": `${R}::${b.key}`,
                ref_for: !0,
                ref: () => {
                }
              }, d(b.render ? "" : y[b.key] ?? "-"), 9, qe)
            ]))), 128))
          ], 2))), 128))
        ])
      ]),
      e.pagination ? (t(), C(Le, {
        key: 0,
        page: e.pagination.page,
        "page-size": e.pagination.pageSize,
        total: e.pagination.total,
        "onUpdate:page": h[1] || (h[1] = (y) => i("update:page", y)),
        "onUpdate:pageSize": h[2] || (h[2] = (y) => i("update:pageSize", y))
      }, null, 8, ["page", "page-size", "total"])) : m("", !0)
    ], 12, Se));
  }
}), xa = /* @__PURE__ */ $(Fe, [["__scopeId", "data-v-6c643f80"]]), Oe = { class: "hnb-page-shell" }, je = { class: "hnb-page-shell__header" }, Ue = { key: 0 }, We = {
  key: 0,
  class: "hnb-page-shell__actions"
}, Ye = { class: "hnb-page-shell__body" }, Ge = /* @__PURE__ */ k({
  __name: "HNBPageShell",
  props: {
    title: {},
    description: {}
  },
  setup(e) {
    return (o, a) => (t(), n("section", Oe, [
      s("header", je, [
        s("div", null, [
          s("h1", null, d(e.title), 1),
          e.description ? (t(), n("p", Ue, d(e.description), 1)) : m("", !0)
        ]),
        o.$slots.actions ? (t(), n("div", We, [
          _(o.$slots, "actions", {}, void 0, !0)
        ])) : m("", !0)
      ]),
      s("div", Ye, [
        _(o.$slots, "default", {}, void 0, !0)
      ])
    ]));
  }
}), Ha = /* @__PURE__ */ $(Ge, [["__scopeId", "data-v-f2815c88"]]), Je = {}, Qe = { class: "hnb-toolbar" }, Xe = { class: "hnb-toolbar__filters" }, Ze = {
  key: 0,
  class: "hnb-toolbar__actions"
};
function et(e, o) {
  return t(), n("div", Qe, [
    s("div", Xe, [
      _(e.$slots, "default", {}, void 0, !0)
    ]),
    e.$slots.actions ? (t(), n("div", Ze, [
      _(e.$slots, "actions", {}, void 0, !0)
    ])) : m("", !0)
  ]);
}
const Ia = /* @__PURE__ */ $(Je, [["render", et], ["__scopeId", "data-v-d53ac2b0"]]), tt = { class: "hnb-table-actions" }, at = /* @__PURE__ */ k({
  __name: "HNBTableActions",
  props: {
    actions: {}
  },
  emits: ["action"],
  setup(e, { emit: o }) {
    const a = o;
    return (i, l) => (t(), n("div", tt, [
      (t(!0), n(H, null, S(e.actions, (c) => (t(), C(D, {
        key: c.key,
        variant: c.variant ?? "ghost",
        size: "small",
        disabled: c.disabled,
        onClick: (g) => a("action", c.key)
      }, {
        default: I(() => [
          K(d(c.label), 1)
        ]),
        _: 2
      }, 1032, ["variant", "disabled", "onClick"]))), 128))
    ]));
  }
}), Na = /* @__PURE__ */ $(at, [["__scopeId", "data-v-d65e4ddf"]]), F = Symbol("hnb-form-field"), nt = ["value", "disabled", "aria-describedby"], lt = {
  value: "",
  disabled: ""
}, it = ["value", "disabled"], st = /* @__PURE__ */ k({
  __name: "HNBSelectInput",
  props: {
    modelValue: { default: "" },
    options: {},
    placeholder: { default: "请选择" },
    disabled: { type: Boolean, default: !1 }
  },
  emits: ["update:modelValue"],
  setup(e, { emit: o }) {
    const a = o, i = j(F, void 0), l = L(() => i == null ? void 0 : i.ariaDescribedBy.value);
    return (c, g) => (t(), n("select", {
      class: "hnb-select-input",
      value: e.modelValue,
      disabled: e.disabled,
      "aria-describedby": l.value,
      onChange: g[0] || (g[0] = (f) => a("update:modelValue", f.target.value))
    }, [
      s("option", lt, d(e.placeholder), 1),
      (t(!0), n(H, null, S(e.options, (f) => (t(), n("option", {
        key: f.value,
        value: f.value,
        disabled: f.disabled
      }, d(f.label), 9, it))), 128))
    ], 40, nt));
  }
}), La = /* @__PURE__ */ $(st, [["__scopeId", "data-v-77ccdfb7"]]), ot = ["type", "value", "disabled", "aria-describedby"], dt = /* @__PURE__ */ k({
  __name: "HNBDateInput",
  props: {
    modelValue: { default: "" },
    type: { default: "date" },
    disabled: { type: Boolean, default: !1 }
  },
  emits: ["update:modelValue"],
  setup(e, { emit: o }) {
    const a = o, i = j(F, void 0), l = L(() => i == null ? void 0 : i.ariaDescribedBy.value);
    return (c, g) => (t(), n("input", {
      class: "hnb-date-input",
      type: e.type,
      value: e.modelValue,
      disabled: e.disabled,
      "aria-describedby": l.value,
      onInput: g[0] || (g[0] = (f) => a("update:modelValue", f.target.value))
    }, null, 40, ot));
  }
}), Sa = /* @__PURE__ */ $(dt, [["__scopeId", "data-v-7a6cb167"]]), rt = ["for"], ct = { class: "hnb-form-field__label" }, ut = {
  key: 0,
  "aria-hidden": "true"
}, bt = ["id"], ft = ["id"], mt = /* @__PURE__ */ k({
  __name: "HNBFormField",
  props: {
    label: {},
    inputId: {},
    help: {},
    error: {},
    required: { type: Boolean }
  },
  setup(e) {
    const o = e, a = L(() => {
      if (o.inputId) {
        if (o.error) return `${o.inputId}-error`;
        if (o.help) return `${o.inputId}-help`;
      }
    });
    return ee(F, { ariaDescribedBy: a }), (i, l) => (t(), n("label", {
      class: "hnb-form-field",
      for: e.inputId
    }, [
      s("span", ct, [
        K(d(e.label), 1),
        e.required ? (t(), n("span", ut, " *")) : m("", !0)
      ]),
      _(i.$slots, "default", {}, void 0, !0),
      e.error ? (t(), n("span", {
        key: 0,
        id: e.inputId ? `${e.inputId}-error` : void 0,
        class: "hnb-form-field__error",
        role: "alert"
      }, d(e.error), 9, bt)) : e.help ? (t(), n("span", {
        key: 1,
        id: e.inputId ? `${e.inputId}-help` : void 0,
        class: "hnb-form-field__help"
      }, d(e.help), 9, ft)) : m("", !0)
    ], 8, rt));
  }
}), Ra = /* @__PURE__ */ $(mt, [["__scopeId", "data-v-39dc2d50"]]), ht = { class: "hnb-detail-panel" }, yt = { key: 0 }, vt = /* @__PURE__ */ k({
  __name: "HNBDetailPanel",
  props: {
    title: {},
    items: {}
  },
  setup(e) {
    return (o, a) => (t(), n("section", ht, [
      e.title ? (t(), n("h2", yt, d(e.title), 1)) : m("", !0),
      s("dl", null, [
        (t(!0), n(H, null, S(e.items, (i) => (t(), n(H, {
          key: i.label
        }, [
          s("dt", null, d(i.label), 1),
          s("dd", null, d(i.value ?? "-"), 1)
        ], 64))), 128))
      ])
    ]));
  }
}), Ca = /* @__PURE__ */ $(vt, [["__scopeId", "data-v-752992e6"]]), gt = {}, $t = { class: "hnb-action-bar" };
function kt(e, o) {
  return t(), n("div", $t, [
    _(e.$slots, "default", {}, void 0, !0)
  ]);
}
const Ka = /* @__PURE__ */ $(gt, [["render", kt], ["__scopeId", "data-v-f58a1523"]]), pt = ["data-semantic"], Bt = { class: "status-label" }, wt = /* @__PURE__ */ k({
  __name: "StatusBadge",
  props: {
    label: {},
    semantic: { default: "default" }
  },
  setup(e) {
    const o = e, a = L(() => {
      switch (o.semantic) {
        case "success":
          return "var(--hnb-color-status-success)";
        case "warning":
          return "var(--hnb-color-status-warning)";
        case "error":
          return "var(--hnb-color-status-danger)";
        case "info":
        case "processing":
          return "var(--hnb-color-status-info)";
        default:
          return "var(--hnb-color-text-tertiary)";
      }
    });
    return (i, l) => (t(), n("span", {
      class: "status-badge",
      "data-semantic": e.semantic
    }, [
      s("span", {
        class: z(["status-dot", { pulsing: e.semantic === "processing" }]),
        style: V({ background: a.value })
      }, null, 6),
      s("span", Bt, d(e.label), 1)
    ], 8, pt));
  }
}), _t = /* @__PURE__ */ $(wt, [["__scopeId", "data-v-186c6d47"]]), xt = { class: "metric-card" }, Ht = { class: "metric-title" }, It = { class: "metric-value-row" }, Nt = { class: "metric-value" }, Lt = {
  key: 0,
  class: "metric-unit"
}, St = {
  key: 0,
  class: "metric-description"
}, Rt = /* @__PURE__ */ k({
  __name: "MetricCard",
  props: {
    title: {},
    value: {},
    unit: { default: "" },
    description: { default: "" }
  },
  setup(e) {
    return (o, a) => (t(), n("div", xt, [
      s("div", Ht, d(e.title), 1),
      s("div", It, [
        s("span", Nt, d(e.value), 1),
        e.unit ? (t(), n("span", Lt, d(e.unit), 1)) : m("", !0)
      ]),
      e.description ? (t(), n("div", St, d(e.description), 1)) : m("", !0)
    ]));
  }
}), Ta = /* @__PURE__ */ $(Rt, [["__scopeId", "data-v-231d74cf"]]), Ct = { class: "description-label" }, Kt = { class: "description-value" }, Tt = /* @__PURE__ */ k({
  __name: "DescriptionList",
  props: {
    items: {},
    column: { default: 2 }
  },
  setup(e) {
    return (o, a) => (t(), n("dl", {
      class: "description-list",
      style: V({ gridTemplateColumns: `repeat(${e.column}, 1fr)` })
    }, [
      (t(!0), n(H, null, S(e.items, (i) => (t(), n("div", {
        key: i.label,
        class: "description-item"
      }, [
        s("dt", Ct, d(i.label), 1),
        s("dd", Kt, d(i.value ?? "-"), 1)
      ]))), 128))
    ], 4));
  }
}), Va = /* @__PURE__ */ $(Tt, [["__scopeId", "data-v-629cffa3"]]), Vt = ["aria-label"], Dt = {
  key: 0,
  class: "skeleton-line skeleton-title"
}, Et = /* @__PURE__ */ k({
  __name: "HNBSkeleton",
  props: {
    rows: { default: 3 },
    title: { type: Boolean, default: !1 },
    label: { default: "Loading" },
    variant: { default: "text" }
  },
  setup(e) {
    return (o, a) => (t(), n("div", {
      class: z(["hnb-skeleton", `hnb-skeleton--${e.variant}`]),
      "aria-busy": "true",
      role: "status",
      "aria-label": e.label
    }, [
      e.title ? (t(), n("div", Dt)) : m("", !0),
      (t(!0), n(H, null, S(e.rows, (i) => (t(), n("div", {
        key: i,
        class: "skeleton-line",
        style: V({ width: i === e.rows ? "60%" : "100%" })
      }, null, 4))), 128))
    ], 10, Vt));
  }
}), zt = /* @__PURE__ */ $(Et, [["__scopeId", "data-v-886c8ed6"]]), Mt = ["aria-describedby", "aria-errormessage", "aria-invalid", "aria-busy"], At = { class: "hnb-dialog__header" }, Pt = { class: "hnb-dialog__body" }, qt = {
  key: 1,
  class: "hnb-dialog__footer"
}, Ft = /* @__PURE__ */ k({
  __name: "HNBDialog",
  props: {
    modelValue: { type: Boolean },
    title: {},
    description: {},
    closeLabel: { default: "Close dialog" },
    closeOnBackdrop: { type: Boolean, default: !0 },
    busy: { type: Boolean, default: !1 },
    error: {},
    initialFocus: {}
  },
  emits: ["update:modelValue", "close"],
  setup(e, { emit: o }) {
    const a = e, i = o, l = U(), c = `${l}-title`, g = `${l}-description`, f = `${l}-error`;
    let r = null, p = null, u = "";
    const v = [
      "button:not([disabled])",
      "[href]",
      "input:not([disabled])",
      "select:not([disabled])",
      "textarea:not([disabled])",
      '[tabindex]:not([tabindex="-1"])'
    ].join(",");
    function h(b) {
      r = b;
    }
    function y() {
      a.busy || (i("update:modelValue", !1), i("close"));
    }
    function R(b) {
      if (b.key === "Escape") {
        b.preventDefault(), y();
        return;
      }
      if (b.key !== "Tab" || !r) return;
      const x = Array.from(r.querySelectorAll(v));
      if (x.length === 0) {
        b.preventDefault(), r.focus();
        return;
      }
      const B = x[0], w = x[x.length - 1];
      b.shiftKey && document.activeElement === B ? (b.preventDefault(), w.focus()) : !b.shiftKey && document.activeElement === w && (b.preventDefault(), B.focus());
    }
    return q(() => a.modelValue, async (b) => {
      var x;
      if (b) {
        p = document.activeElement instanceof HTMLElement ? document.activeElement : null, u = document.body.style.overflow, document.body.style.overflow = "hidden", await P();
        const B = a.initialFocus ? r == null ? void 0 : r.querySelector(a.initialFocus) : null, w = r == null ? void 0 : r.querySelector(v);
        (x = B ?? w ?? r) == null || x.focus();
      } else
        document.body.style.overflow = u, p == null || p.focus(), p = null;
    }, { immediate: !0 }), te(() => {
      document.body.style.overflow = u, p == null || p.focus();
    }), (b, x) => (t(), C(ae, { to: "body" }, [
      e.modelValue ? (t(), n("div", {
        key: 0,
        class: "hnb-dialog-layer",
        onMousedown: x[0] || (x[0] = ne((B) => e.closeOnBackdrop && y(), ["self"]))
      }, [
        s("section", {
          ref: h,
          class: "hnb-dialog",
          role: "dialog",
          "aria-modal": "true",
          "aria-labelledby": c,
          "aria-describedby": [e.description ? g : "", e.error ? f : ""].filter(Boolean).join(" ") || void 0,
          "aria-errormessage": e.error ? f : void 0,
          "aria-invalid": e.error ? "true" : void 0,
          "aria-busy": e.busy,
          tabindex: "-1",
          onKeydown: R
        }, [
          s("header", At, [
            s("div", null, [
              s("h2", {
                id: c,
                class: "hnb-dialog__title"
              }, d(e.title), 1),
              e.description ? (t(), n("p", {
                key: 0,
                id: g,
                class: "hnb-dialog__description"
              }, d(e.description), 1)) : m("", !0)
            ]),
            T(D, {
              variant: "ghost",
              size: "small",
              disabled: e.busy,
              "aria-label": e.closeLabel,
              onClick: y
            }, {
              default: I(() => [...x[1] || (x[1] = [
                K("×", -1)
              ])]),
              _: 1
            }, 8, ["disabled", "aria-label"])
          ]),
          s("div", Pt, [
            _(b.$slots, "default", {}, void 0, !0)
          ]),
          e.error ? (t(), n("div", {
            key: 0,
            id: f,
            class: "hnb-dialog__error",
            role: "alert"
          }, d(e.error), 1)) : m("", !0),
          b.$slots.footer ? (t(), n("footer", qt, [
            _(b.$slots, "footer", {}, void 0, !0)
          ])) : m("", !0)
        ], 40, Mt)
      ], 32)) : m("", !0)
    ]));
  }
}), Ot = /* @__PURE__ */ $(Ft, [["__scopeId", "data-v-afacb521"]]), jt = {
  key: 0,
  class: "hnb-confirmation__acknowledgement"
}, Ut = /* @__PURE__ */ k({
  __name: "HNBConfirmation",
  props: {
    modelValue: { type: Boolean },
    title: {},
    description: {},
    confirmText: { default: "Confirm" },
    cancelText: { default: "Cancel" },
    danger: { type: Boolean, default: !1 },
    loading: { type: Boolean, default: !1 },
    error: {},
    requireAcknowledgement: { type: Boolean, default: !1 },
    acknowledgementLabel: { default: "I understand the impact" }
  },
  emits: ["update:modelValue", "confirm", "cancel"],
  setup(e, { emit: o }) {
    const a = e, i = o, l = E(!1);
    q(() => a.modelValue, (g) => {
      g && (l.value = !1);
    });
    function c() {
      a.loading || (i("update:modelValue", !1), i("cancel"));
    }
    return (g, f) => (t(), C(Ot, {
      "model-value": e.modelValue,
      title: e.title,
      description: e.description,
      busy: e.loading,
      error: e.error,
      "initial-focus": "[data-confirm-cancel]",
      "onUpdate:modelValue": f[2] || (f[2] = (r) => i("update:modelValue", r)),
      onClose: f[3] || (f[3] = (r) => i("cancel"))
    }, {
      footer: I(() => [
        T(D, {
          "data-confirm-cancel": "",
          disabled: e.loading,
          onClick: c
        }, {
          default: I(() => [
            K(d(e.cancelText), 1)
          ]),
          _: 1
        }, 8, ["disabled"]),
        T(D, {
          variant: e.danger ? "danger" : "primary",
          loading: e.loading,
          disabled: e.requireAcknowledgement && !l.value,
          "disabled-reason": e.requireAcknowledgement && !l.value ? e.acknowledgementLabel : void 0,
          onClick: f[1] || (f[1] = (r) => i("confirm"))
        }, {
          default: I(() => [
            K(d(e.confirmText), 1)
          ]),
          _: 1
        }, 8, ["variant", "loading", "disabled", "disabled-reason"])
      ]),
      default: I(() => [
        _(g.$slots, "default", {}, void 0, !0),
        e.requireAcknowledgement ? (t(), n("label", jt, [
          W(s("input", {
            "onUpdate:modelValue": f[0] || (f[0] = (r) => l.value = r),
            type: "checkbox"
          }, null, 512), [
            [le, l.value]
          ]),
          s("span", null, d(e.acknowledgementLabel), 1)
        ])) : m("", !0)
      ]),
      _: 3
    }, 8, ["model-value", "title", "description", "busy", "error"]));
  }
}), Da = /* @__PURE__ */ $(Ut, [["__scopeId", "data-v-47d55497"]]), Wt = ["role", "aria-live"], Yt = { class: "hnb-alert__content" }, Gt = {
  key: 0,
  class: "hnb-alert__title"
}, Jt = { class: "hnb-alert__body" }, Qt = {
  key: 1,
  class: "hnb-alert__actions"
}, Xt = /* @__PURE__ */ k({
  __name: "HNBAlert",
  props: {
    title: {},
    semantic: { default: "info" },
    live: { default: "polite" },
    dismissLabel: { default: "Dismiss" },
    dismissible: { type: Boolean, default: !1 }
  },
  emits: ["dismiss"],
  setup(e, { emit: o }) {
    const a = o;
    return (i, l) => (t(), n("div", {
      class: z(["hnb-alert", `hnb-alert--${e.semantic}`]),
      role: e.live === "assertive" ? "alert" : "status",
      "aria-live": e.live,
      "aria-atomic": "true"
    }, [
      l[2] || (l[2] = s("span", {
        class: "hnb-alert__marker",
        "aria-hidden": "true"
      }, "!", -1)),
      s("div", Yt, [
        e.title ? (t(), n("div", Gt, d(e.title), 1)) : m("", !0),
        s("div", Jt, [
          _(i.$slots, "default", {}, void 0, !0)
        ]),
        i.$slots.actions ? (t(), n("div", Qt, [
          _(i.$slots, "actions", {}, void 0, !0)
        ])) : m("", !0)
      ]),
      e.dismissible ? (t(), C(D, {
        key: 0,
        variant: "ghost",
        size: "small",
        "aria-label": e.dismissLabel,
        onClick: l[0] || (l[0] = (c) => a("dismiss"))
      }, {
        default: I(() => [...l[1] || (l[1] = [
          K("×", -1)
        ])]),
        _: 1
      }, 8, ["aria-label"])) : m("", !0)
    ], 10, Wt));
  }
}), Q = /* @__PURE__ */ $(Xt, [["__scopeId", "data-v-a8692e37"]]), Zt = { class: "hnb-tabs" }, ea = ["aria-label"], ta = ["id", "aria-selected", "aria-controls", "aria-describedby", "tabindex", "disabled", "onClick", "onKeydown"], aa = ["id"], na = ["id", "aria-labelledby"], la = /* @__PURE__ */ k({
  __name: "HNBTabs",
  props: {
    modelValue: {},
    tabs: {},
    ariaLabel: {}
  },
  emits: ["update:modelValue"],
  setup(e, { emit: o }) {
    const a = e, i = o, l = U();
    function c() {
      return a.tabs.filter((r) => !r.disabled);
    }
    async function g(r) {
      var p;
      r.disabled || (i("update:modelValue", r.id), await P(), (p = document.getElementById(`${l}-tab-${r.id}`)) == null || p.focus());
    }
    function f(r, p) {
      const u = c(), v = u.findIndex((y) => y.id === p.id);
      let h;
      r.key === "ArrowRight" && (h = u[(v + 1) % u.length]), r.key === "ArrowLeft" && (h = u[(v - 1 + u.length) % u.length]), r.key === "Home" && (h = u[0]), r.key === "End" && (h = u[u.length - 1]), h && (r.preventDefault(), g(h));
    }
    return (r, p) => (t(), n("div", Zt, [
      s("div", {
        class: "hnb-tabs__list",
        role: "tablist",
        "aria-label": e.ariaLabel
      }, [
        (t(!0), n(H, null, S(e.tabs, (u) => (t(), n(H, {
          key: u.id
        }, [
          s("button", {
            id: `${M(l)}-tab-${u.id}`,
            class: "hnb-tabs__tab",
            type: "button",
            role: "tab",
            "aria-selected": e.modelValue === u.id,
            "aria-controls": `${M(l)}-panel-${u.id}`,
            "aria-describedby": u.disabledReason ? `${M(l)}-reason-${u.id}` : void 0,
            tabindex: e.modelValue === u.id ? 0 : -1,
            disabled: u.disabled,
            onClick: (v) => g(u),
            onKeydown: (v) => f(v, u)
          }, d(u.label), 41, ta),
          u.disabledReason ? (t(), n("span", {
            key: 0,
            id: `${M(l)}-reason-${u.id}`,
            class: "hnb-tabs__sr-only"
          }, d(u.disabledReason), 9, aa)) : m("", !0)
        ], 64))), 128))
      ], 8, ea),
      (t(!0), n(H, null, S(e.tabs, (u) => W((t(), n("div", {
        id: `${M(l)}-panel-${u.id}`,
        key: `${u.id}-panel`,
        class: "hnb-tabs__panel",
        role: "tabpanel",
        "aria-labelledby": `${M(l)}-tab-${u.id}`,
        tabindex: "0"
      }, [
        _(r.$slots, `panel-${u.id}`, {}, void 0, !0)
      ], 8, na)), [
        [ie, e.modelValue === u.id]
      ])), 128))
    ]));
  }
}), Ea = /* @__PURE__ */ $(la, [["__scopeId", "data-v-59b35b83"]]), ia = ["aria-label"], sa = { class: "hnb-status-group__label" }, oa = {
  key: 0,
  class: "hnb-status-group__last-known"
}, da = /* @__PURE__ */ k({
  __name: "HNBStatusGroup",
  props: {
    items: {},
    ariaLabel: {},
    lastKnownLabel: {}
  },
  setup(e) {
    return (o, a) => (t(), n("div", {
      class: "hnb-status-group",
      role: "group",
      "aria-label": e.ariaLabel
    }, [
      (t(!0), n(H, null, S(e.items, (i) => (t(), n("div", {
        key: i.key,
        class: "hnb-status-group__item"
      }, [
        s("span", sa, d(i.label), 1),
        T(_t, {
          label: i.valueLabel,
          semantic: i.semantic
        }, null, 8, ["label", "semantic"])
      ]))), 128)),
      e.lastKnownLabel ? (t(), n("div", oa, d(e.lastKnownLabel), 1)) : m("", !0)
    ], 8, ia));
  }
}), za = /* @__PURE__ */ $(da, [["__scopeId", "data-v-4012af65"]]), ra = ["data-state", "aria-label"], ca = /* @__PURE__ */ k({
  __name: "HNBPageState",
  props: {
    state: {},
    title: {},
    description: {},
    actionText: {},
    actionLoading: { type: Boolean, default: !1 },
    skeletonRows: { default: 3 }
  },
  emits: ["action"],
  setup(e, { emit: o }) {
    const a = o;
    return (i, l) => (t(), n("section", {
      class: "hnb-page-state",
      "data-state": e.state,
      "aria-label": e.title
    }, [
      e.state === "loading" ? (t(), C(zt, {
        key: 0,
        rows: e.skeletonRows,
        label: e.title,
        title: ""
      }, null, 8, ["rows", "label"])) : e.state === "empty" ? (t(), C(Y, {
        key: 1,
        title: e.title,
        description: e.description,
        "action-text": e.actionText,
        onAction: l[0] || (l[0] = (c) => a("action"))
      }, null, 8, ["title", "description", "action-text"])) : e.state === "error" ? (t(), C(G, {
        key: 2,
        title: e.title,
        description: e.description,
        "retry-text": e.actionText || "Retry",
        "retry-loading": e.actionLoading,
        onRetry: l[1] || (l[1] = (c) => a("action"))
      }, null, 8, ["title", "description", "retry-text", "retry-loading"])) : (t(), C(Q, {
        key: 3,
        semantic: e.state === "offline" ? "warning" : "info",
        live: e.state === "offline" ? "assertive" : "polite",
        title: e.title
      }, se({
        default: I(() => [
          K(d(e.description) + " ", 1)
        ]),
        _: 2
      }, [
        e.actionText ? {
          name: "actions",
          fn: I(() => [
            T(D, {
              loading: e.actionLoading,
              onClick: l[2] || (l[2] = (c) => a("action"))
            }, {
              default: I(() => [
                K(d(e.actionText), 1)
              ]),
              _: 1
            }, 8, ["loading"])
          ]),
          key: "0"
        } : void 0
      ]), 1032, ["semantic", "live", "title"]))
    ], 8, ra));
  }
}), Ma = /* @__PURE__ */ $(ca, [["__scopeId", "data-v-f0f89c4c"]]), ua = ["aria-label"], ba = ["aria-label", "aria-valuenow"], fa = { class: "hnb-operation-progress__steps" }, ma = ["data-status", "aria-current"], ha = {
  class: "hnb-operation-progress__marker",
  "aria-hidden": "true"
}, ya = { class: "hnb-operation-progress__label" }, va = {
  key: 0,
  class: "hnb-operation-progress__description"
}, ga = { key: 1 }, $a = /* @__PURE__ */ k({
  __name: "HNBOperationProgress",
  props: {
    label: {},
    steps: {},
    value: {},
    statusMessage: {}
  },
  setup(e) {
    const o = e, a = L(() => o.value === void 0 ? void 0 : Math.min(100, Math.max(0, o.value)));
    return (i, l) => (t(), n("section", {
      class: "hnb-operation-progress",
      "aria-label": e.label
    }, [
      s("div", {
        class: "hnb-operation-progress__bar",
        role: "progressbar",
        "aria-label": e.label,
        "aria-valuemin": "0",
        "aria-valuemax": "100",
        "aria-valuenow": a.value
      }, [
        s("div", {
          class: z(["hnb-operation-progress__fill", { "is-indeterminate": a.value === void 0 }]),
          style: V(a.value === void 0 ? void 0 : { width: `${a.value}%` })
        }, null, 6)
      ], 8, ba),
      s("ol", fa, [
        (t(!0), n(H, null, S(e.steps, (c) => (t(), n("li", {
          key: c.id,
          class: "hnb-operation-progress__step",
          "data-status": c.status,
          "aria-current": c.status === "running" ? "step" : void 0
        }, [
          s("span", ha, d(c.status === "success" ? "✓" : c.status === "error" ? "!" : "•"), 1),
          s("div", null, [
            s("div", ya, d(c.label), 1),
            c.description ? (t(), n("div", va, d(c.description), 1)) : m("", !0),
            c.timestamp ? (t(), n("time", ga, d(c.timestamp), 1)) : m("", !0)
          ])
        ], 8, ma))), 128))
      ]),
      e.statusMessage ? (t(), C(J, {
        key: 0,
        message: e.statusMessage
      }, null, 8, ["message"])) : m("", !0)
    ], 8, ua));
  }
}), Aa = /* @__PURE__ */ $($a, [["__scopeId", "data-v-dee792c8"]]), ka = {
  key: 0,
  class: "hnb-virtual-list-loading"
}, pa = {
  key: 1,
  class: "hnb-virtual-list-empty"
}, Ba = /* @__PURE__ */ k({
  __name: "HNBVirtualList",
  props: {
    data: {},
    itemHeight: {},
    height: {},
    rowKey: {},
    loading: { type: Boolean, default: !1 },
    overscan: { default: 5 }
  },
  emits: ["endReached"],
  setup(e, { emit: o }) {
    const a = e, i = o, l = E(null), c = E(0), g = E(0);
    function f(B, w) {
      if (!a.rowKey || typeof B != "object" || B === null) return w;
      const N = B[a.rowKey];
      return typeof N == "string" || typeof N == "number" || typeof N == "symbol" ? N : w;
    }
    const r = L(() => Math.max(0, Math.floor(c.value / a.itemHeight) - a.overscan)), p = L(() => Math.min(a.data.length, Math.ceil((c.value + g.value) / a.itemHeight) + a.overscan)), u = L(() => a.data.slice(r.value, p.value)), v = L(() => r.value * a.itemHeight), h = L(() => a.data.length * a.itemHeight), y = L(() => c.value + g.value >= a.data.length * a.itemHeight - a.itemHeight * 2);
    let R = -1;
    q([y, () => a.loading, () => a.data.length], ([B, w, N]) => {
      B && !w && N > 0 && N !== R && (R = N, i("endReached"));
    });
    let b = null;
    O(() => {
      l.value && (g.value = l.value.clientHeight, b = new ResizeObserver(() => {
        var B;
        g.value = ((B = l.value) == null ? void 0 : B.clientHeight) ?? 0;
      }), b.observe(l.value));
    }), oe(() => {
      b == null || b.disconnect();
    });
    function x() {
      l.value && (c.value = l.value.scrollTop);
    }
    return (B, w) => (t(), n("div", {
      ref_key: "containerEl",
      ref: l,
      class: "hnb-virtual-list",
      style: V({ height: e.height ? e.height + "px" : "100%" }),
      onScroll: x
    }, [
      s("div", {
        class: "hnv-virtual-list-spacer",
        style: V({ height: h.value + "px" })
      }, [
        s("div", {
          class: "hnb-virtual-list-items",
          style: V({ transform: `translateY(${v.value}px)` })
        }, [
          (t(!0), n(H, null, S(u.value, (N, A) => (t(), n("div", {
            key: f(N, A),
            class: "hnb-virtual-list-item",
            style: V({ height: e.itemHeight + "px" })
          }, [
            _(B.$slots, "item", {
              item: N,
              index: r.value + A
            }, void 0, !0)
          ], 4))), 128))
        ], 4)
      ], 4),
      e.loading ? (t(), n("div", ka, [
        _(B.$slots, "loading", {}, () => [
          w[0] || (w[0] = s("span", null, "加载中...", -1))
        ], !0)
      ])) : m("", !0),
      !e.loading && e.data.length === 0 ? (t(), n("div", pa, [
        _(B.$slots, "empty", {}, () => [
          w[1] || (w[1] = s("span", null, "暂无数据", -1))
        ], !0)
      ])) : m("", !0)
    ], 36));
  }
}), Pa = /* @__PURE__ */ $(Ba, [["__scopeId", "data-v-ee9a4ebb"]]), wa = /* @__PURE__ */ k({
  __name: "HNBErrorBoundary",
  props: {
    title: { default: "区块异常" },
    description: { default: "渲染时发生错误，请重试。" },
    retryText: { default: "重试" }
  },
  setup(e) {
    const o = E(!1), a = E(0);
    de((l) => (o.value = !0, console.warn("[HNBErrorBoundary] caught:", l), !1));
    function i() {
      o.value = !1, a.value++;
    }
    return (l, c) => o.value ? (t(), C(Q, {
      key: 0,
      semantic: "error",
      title: e.title,
      description: e.description
    }, {
      action: I(() => [
        s("button", {
          class: "hnb-error-retry",
          type: "button",
          onClick: i
        }, d(e.retryText), 1)
      ]),
      _: 1
    }, 8, ["title", "description"])) : _(l.$slots, "default", {}, void 0, !0, 1);
  }
}), qa = /* @__PURE__ */ $(wa, [["__scopeId", "data-v-4e8b568a"]]);
export {
  Va as DescriptionList,
  Y as EmptyState,
  G as ErrorState,
  Ka as HNBActionBar,
  Q as HNBAlert,
  D as HNBButton,
  Da as HNBConfirmation,
  Sa as HNBDateInput,
  Ca as HNBDetailPanel,
  Ot as HNBDialog,
  qa as HNBErrorBoundary,
  Ra as HNBFormField,
  J as HNBLiveRegion,
  Aa as HNBOperationProgress,
  Ha as HNBPageShell,
  Ma as HNBPageState,
  Le as HNBPagination,
  La as HNBSelectInput,
  zt as HNBSkeleton,
  za as HNBStatusGroup,
  xa as HNBTable,
  Na as HNBTableActions,
  Ea as HNBTabs,
  Ia as HNBToolbar,
  Pa as HNBVirtualList,
  F as HNB_FORM_FIELD_INJECTION_KEY,
  Ta as MetricCard,
  _t as StatusBadge
};
