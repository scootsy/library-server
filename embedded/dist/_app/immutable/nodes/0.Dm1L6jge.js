import{d as J,c as T,a as i,f as d,s as C,b as K}from"../chunks/D4wxxk7-.js";import{o as O}from"../chunks/Ctlg7veE.js";import{k as V,al as W,a0 as X,a2 as A,a3 as Y,a4 as f,m as s,a6 as q,a7 as r,a1 as h,aa as Z,a8 as l,a9 as F}from"../chunks/DZbbceta.js";import{B as $,i as _}from"../chunks/CIt3n3Sj.js";import{h as ee,v as ae,w as se}from"../chunks/PIbT5kSg.js";function L(b,y,...n){var u=new $(b);V(()=>{const c=y()??null;u.ensure(c,c&&(g=>c(g,...n)))},W)}var te=d(`<style>:root {
			--bg: #0f1117;
			--bg-surface: #1a1d27;
			--bg-hover: #232736;
			--border: #2a2e3d;
			--text: #e4e5e9;
			--text-muted: #8b8fa3;
			--primary: #6c8cff;
			--primary-hover: #8ba3ff;
			--danger: #ef4444;
			--success: #22c55e;
			--warning: #f59e0b;
			--radius: 6px;
		}

		* { box-sizing: border-box; margin: 0; padding: 0; }

		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
			background: var(--bg);
			color: var(--text);
			line-height: 1.5;
		}

		a { color: var(--primary); text-decoration: none; }
		a:hover { color: var(--primary-hover); }</style>`),re=d('<div class="loading-screen svelte-12qhfyh"><div class="loading-spinner svelte-12qhfyh"></div></div>'),le=d('<li class="svelte-12qhfyh"><a href="/users" class="svelte-12qhfyh">Users</a></li>'),oe=d('<div class="layout svelte-12qhfyh"><nav class="sidebar svelte-12qhfyh"><div class="logo svelte-12qhfyh"><h1 class="svelte-12qhfyh">Codex</h1></div> <ul class="nav-links svelte-12qhfyh"><li class="svelte-12qhfyh"><a href="/" class="svelte-12qhfyh">Dashboard</a></li> <li class="svelte-12qhfyh"><a href="/browse" class="svelte-12qhfyh">Browse</a></li> <li class="svelte-12qhfyh"><a href="/review" class="svelte-12qhfyh">Review Queue</a></li> <li class="svelte-12qhfyh"><a href="/collections" class="svelte-12qhfyh">Collections</a></li> <!> <li class="svelte-12qhfyh"><a href="/settings" class="svelte-12qhfyh">Settings</a></li></ul> <div class="sidebar-footer svelte-12qhfyh"><div class="user-info svelte-12qhfyh"><span class="user-name svelte-12qhfyh"> </span> <span class="user-role svelte-12qhfyh"> </span></div> <button class="btn-logout svelte-12qhfyh">Sign out</button></div></nav> <main class="content svelte-12qhfyh"><!></main></div>'),ie=d('<p class="auth-error svelte-12qhfyh"> </p>'),ne=d('<p><a href="/login">Go to login</a></p>'),ve=d('<div class="unauthenticated-screen svelte-12qhfyh"><p>Authentication required. Redirecting to login…</p> <!> <!></div>');function ye(b,y){X(y,!0);let n=q(null),u=q(!1),c=q(!1),g=q(""),S=q(!1);O(()=>{if(f(c,window.location.pathname==="/login"),s(c)){f(u,!0);return}ae().then(e=>{f(n,e,!0),f(u,!0)}).catch(e=>{f(g,(e==null?void 0:e.message)||"authentication required",!0),f(u,!0),f(S,!0),window.location.href="/login"})});async function M(){try{await se()}catch{}window.location.href="/login"}var B=T();ee("12qhfyh",e=>{var a=te();i(e,a)});var U=A(B);{var N=e=>{var a=re();i(e,a)},P=e=>{var a=T(),v=A(a);L(v,()=>y.children),i(e,a)},z=e=>{var a=oe(),v=r(a),p=h(r(v),2),w=h(r(p),8);{var x=k=>{var H=le();i(k,H)};_(w,k=>{s(n).role==="admin"&&k(x)})}Z(2),l(p);var t=h(p,2),o=r(t),m=r(o),G=r(m,!0);l(m);var E=h(m,2),I=r(E,!0);l(E),l(o);var Q=h(o,2);l(t),l(v);var R=h(v,2),j=r(R);L(j,()=>y.children),l(R),l(a),F(()=>{C(G,s(n).display_name||s(n).username),C(I,s(n).role)}),K("click",Q,M),i(e,a)},D=e=>{var a=ve(),v=h(r(a),2);{var p=t=>{var o=ie(),m=r(o,!0);l(o),F(()=>C(m,s(g))),i(t,o)};_(v,t=>{s(g)&&t(p)})}var w=h(v,2);{var x=t=>{var o=ne();i(t,o)};_(w,t=>{s(S)&&t(x)})}l(a),i(e,a)};_(U,e=>{s(u)?s(c)?e(P,1):s(n)?e(z,2):e(D,-1):e(N)})}i(b,B),Y()}J(["click"]);export{ye as component};
