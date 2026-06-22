# 机器兔主角色 · 状态立绘提示词（蓝黑赛博 · 大屏指挥台用）

> 用途：大屏「项目总表」里每个工位坐着的就是这只机器兔（agent 画像），形态跟任务状态平级——
> 忙 / 阻塞 / 庆祝 / 休息。本文件是一套**同角色、同机位、只换姿态+表情+光效**的状态立绘提示词。
>
> 参考图：`publics/beginner_副本.png`（蓝色调，贴大屏蓝黑）或 `publics/advance_user.jpeg`（坐在操控台构图）。
>
> 关键一致性：5 张必须**同一机位、同一缩放、居中、背景透明**，只有姿态/表情/霓虹光效/周围全息随状态变，
> 这样在工位里换 PNG 才不会跳；小到 56px 也要能一眼读出状态。
>
> 用法：每条 prompt 都挂同一张参考图。Midjourney 把末尾 `--cref <参考图URL> --cw 90` 换成图链；
> 即梦/可灵等上传参考图并开「保持角色一致性」。下面每个代码块都是**完整可直接复制**的，无需再拼接。

---

## ① 待机 / 休息（当下无任务在执行）

```
A chubby cute cyber robot rabbit mascot, glossy white-and-silver armored panels, two long upright segmented mechanical ears with glowing neon tips and thin antennae, a wide horizontal visor across the eyes with a small glowing rabbit emblem, round chest panel with a circular core light, headphone-like side units, sitting cross-legged at a small futuristic console with a keyboard, glowing circular pedestal base with cables. Pixar/Blender-style 3D render, soft studio lighting, clean, high detail, centered, 3/4 front view. State: idle / standby — relaxed posture, hands resting off the keyboard, visor dimmed to a soft low blue, half-closed sleepy eyes, ears slightly drooping, a faint "Zzz" hologram floating above the head, minimal dim ambient glow. Isolated character, transparent background, identical camera and scale across the set, deep blue-black cyberpunk palette, electric-blue neon accents. --ar 1:1 --cref <参考图URL> --cw 90
```

## ② 工作中 / 忙（有任务正在执行）

```
A chubby cute cyber robot rabbit mascot, glossy white-and-silver armored panels, two long upright segmented mechanical ears with glowing neon tips and thin antennae, a wide horizontal visor across the eyes with a small glowing rabbit emblem, round chest panel with a circular core light, headphone-like side units, sitting cross-legged at a small futuristic console with a keyboard, glowing circular pedestal base with cables. Pixar/Blender-style 3D render, soft studio lighting, clean, high detail, centered, 3/4 front view. State: busy / focused working — both hands actively typing on the keyboard, bright cyan-blue visor glowing, alert focused eyes, ears perked upright, floating holographic terminal windows around it with scrolling code and progress bars, energetic electric-blue neon. Isolated character, transparent background, identical camera and scale across the set, deep blue-black cyberpunk palette, electric-blue neon accents. --ar 1:1 --cref <参考图URL> --cw 90
```

## ③ 遇到问题 / 阻塞（任务阻塞或失败）

```
A chubby cute cyber robot rabbit mascot, glossy white-and-silver armored panels, two long upright segmented mechanical ears with glowing neon tips and thin antennae, a wide horizontal visor across the eyes with a small glowing rabbit emblem, round chest panel with a circular core light, headphone-like side units, sitting cross-legged at a small futuristic console with a keyboard, glowing circular pedestal base with cables. Pixar/Blender-style 3D render, soft studio lighting, clean, high detail, centered, 3/4 front view. State: blocked / alert — one hand raised, head tilted, worried expression, visor flashing warning RED, red holographic error and lock windows with a red "!" alert icon floating nearby, sparks, red emergency glow replacing the blue. Isolated character, transparent background, identical camera and scale across the set, deep blue-black cyberpunk palette, red emergency neon accents. --ar 1:1 --cref <参考图URL> --cw 90
```

## ④ 庆祝 / 完成（任务全部完成）

```
A chubby cute cyber robot rabbit mascot, glossy white-and-silver armored panels, two long upright segmented mechanical ears with glowing neon tips and thin antennae, a wide horizontal visor across the eyes with a small glowing rabbit emblem, round chest panel with a circular core light, headphone-like side units, sitting cross-legged at a small futuristic console with a keyboard, glowing circular pedestal base with cables. Pixar/Blender-style 3D render, soft studio lighting, clean, high detail, centered, 3/4 front view. State: celebrating / success — both arms raised up joyfully, big happy eyes, ears perked, visor glowing warm GOLD, confetti and golden sparkles bursting around, a small "DONE 100%" hologram, triumphant cheerful mood. Isolated character, transparent background, identical camera and scale across the set, deep blue-black cyberpunk palette, gold-and-blue neon accents. --ar 1:1 --cref <参考图URL> --cw 90
```

## ⑤ 发射 / 待交付（英雄时刻 · 可选）

```
A chubby cute cyber robot rabbit mascot, glossy white-and-silver armored panels, two long upright segmented mechanical ears with glowing neon tips and thin antennae, a wide horizontal visor across the eyes with a small glowing rabbit emblem, round chest panel with a circular core light, headphone-like side units, glowing circular pedestal base with cables. Pixar/Blender-style 3D render, soft studio lighting, clean, high detail, centered, 3/4 front view. State: launch / hero shot — confident heroic standing pose pointing forward, holding or beside a small glowing rocket, golden-and-blue neon, dynamic light streaks, a "BUILDING IN PUBLIC" hologram, cinematic dramatic lighting, the most shareable shot. Isolated character, transparent background, identical camera and scale across the set, deep blue-black cyberpunk palette, gold-and-blue neon accents. --ar 1:1 --cref <参考图URL> --cw 90
```

---

## 状态 → 任务状态映射（落地时对回真实 primitive）

| 立绘状态 | 触发条件（任务聚合状态） | 大屏分组 |
|---|---|---|
| ② 忙 | 有任务正在执行 | 进行中 |
| ③ 阻塞 | 任意任务 blocked / failed | 遇到问题（排最前 + 脉冲） |
| ④ 庆祝 | 全部任务完成 | 已完成 |
| ① 休息 | 当下无任务执行、但未全完成 | 休息中（降亮度） |
| ⑤ 发射 | 已交付、可发射（英雄时刻 / 截图位） | 已完成 → 发射动作 |
