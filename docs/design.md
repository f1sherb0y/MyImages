# MyImages 的设计

MyImages 是用 [MyGo](https://github.com/egoist/mygo) 原生界面写的看图软件，目标是比 ImageGlass 10 快、省：启动快，切图快，界面紧凑，占的 CPU、内存、磁盘都少。

## 分层

```
main.go              入口：解析命令行
internal/app         界面：窗口、图片区、工具栏、缩略图条、信息面板、设置、更新（全平台同一份）
internal/codec       解码：文件 → 像素。系统解码器（WIC）、Go 的解码库、兜底
internal/folder      文件夹里有哪些图片，按什么顺序（自然排序）
internal/shell       桌面：回收站、剪贴板里的图片、资源管理器、壁纸、打印
internal/config      设置（JSON，在用户配置目录）
locales/             界面文字：en、zh-Hans（MyGo 的多语言机制，catalog_gen.go 是生成的）
third_party/mygo     MyGo 0.9.0，带一个小补丁（patches/mygo.patch）
tools/               startbench（秒表）、decbench（解码器计时）
```

依赖只往下走：`app` 用其余四个包，它们互不依赖，也不知道界面。和操作系统打交道的代码都在 `*_windows.go` 里，全部用 `syscall`，没有 cgo；其他系统有能编译的桩。

## 为什么快

### 启动

一次启动按时间顺序（这台机器，`MYIMAGES_TRACE=1` 可以看到）：

1. **0 ms**：`main` 一开始就做三件互不等待的事：在另一个线程上创建 Direct3D 设备（加载显卡驱动要 100–200 ms，`ui.Prewarm`，补丁）；开始解码命令行给的那张图；启动界面循环。文件夹的列表也在另一个线程上读，先只显示这一张。
2. **~15 ms**：界面循环就绪，建窗口。
3. **~70 ms**：图解码完（按窗口大小缩小解码，见下）。
4. **~115 ms**：窗口的第一帧。这一帧会等图最多 250 ms，所以第一帧里就有图，而不是先出一个空窗口。如果这时显卡设备还没好，这一帧就在内存里画（补丁），不等它；设备好了之后的帧才走 GPU。两种画法的像素相同（逐像素比过）。
5. 系统把窗口显示出来。

没有启动画面，没有先于图片出现的东西。窗口图标在第一帧之后才设。更新检查在第一帧之后的另一个线程上。

### 只解码看得见的像素

一张 3200 万像素的照片全尺寸解码要 200 ms、128 MB，而窗口里只显示得下一两百万像素。所以：

- **预览**：先按图片区的像素大小解码。JPEG 解码器自己能按 1/2、1/4、1/8 解（WIC 的 `IWICBitmapSourceTransform`），3200 万像素的图只要 50 ms；别的格式解完再缩小。图片区的大小记在设置里，所以窗口还没建好就知道该解多大。
- **全尺寸**：只有当用户放大到超过预览的分辨率时才解码（`viewer.go` 的 `paint` 里判断），解好之前先显示放大的预览。大多数图片只是看一眼就翻过去，从不需要全尺寸。ImageGlass 每张都全尺寸解码，这是内存差 9 倍的主要原因。
- **金字塔**：全尺寸解好后在解码线程上顺手做出逐级减半的几层，缩小显示时取刚好够用的一层，不闪烁，也不在主线程上现算。
- **分块上传**：像素切成约 16 MB 的横条交给 GPU（宽过 8192 的再切成列），显示到哪块才上传哪块，所以再大的图也不会有一帧卡在上传上，也不受纹理尺寸上限的限制。横条直接引用解码出来的内存，不复制。

### 切图

- **预读**：显示一张图时，在后台解码前进方向的后 3 张和反方向的 1 张的预览，近的先做；翻到时已经在内存里，这一帧就画出来，所以是 25 ms（其中大部分是等下一次屏幕刷新）。前后各留 4 张，更远的丢掉。
- **不等旧图**：新图的预览还没好时继续显示上一张，好了就换；连续按键时来不及的图直接跳过，排队的解码按「正在看的优先、后请求的先做」，被翻过去的取消。
- **缩略图条**是自己画的一行，不是每张一个控件；一万张图的文件夹里只为看得见的那十几张取缩略图（相机内嵌的缩略图够大就直接用，否则缩小解码）。

### 省

- 空闲时不画帧（MyGo 只在有变化时画），CPU 为零。
- 解码告一段落 2 秒后把用过的内存还给系统。
- 一个 23.7 MB 的 exe，没有运行时、没有附带的 DLL。

## 格式

规则：只用 Windows 自带的解码器、编进 exe 的 Go 库，没有库的才自己写；不调用任何别的程序，也不假设用户装了什么。系统里装了更快的解码器（HEIF、AVIF、WebP、JPEG XL、RAW 的扩展）就用，没装时每一种都有进程内的替补。

| 格式 | 扩展名 | 谁解码 |
|---|---|---|
| JPEG、PNG、BMP、TIFF、ICO | jpg jpeg jpe jfif jif exif · png apng · bmp dib rle · tif tiff · ico cur | Windows 自带的 WIC；没有时是 Go 标准库和 golang.org/x/image |
| GIF | gif gifv | Go 标准库（动图由 MyGo 合成） |
| WebP | webp | 系统的扩展，或 gen2brain/webp（动图总是它） |
| HEIF | heic heif hif heics heifs avci avcs | 系统的扩展，或 gen2brain/heic |
| AVIF | avif avifs | 系统的扩展，或 gen2brain/avif |
| JPEG XL | jxl | 系统的扩展，或 gen2brain/jpegxl |
| JPEG 2000 | jp2 j2k j2c jpc jpf jpx jpm | ajroetker/go-jpeg2000 |
| JPEG XR | jxr hdp wdp | WIC 自带 |
| DDS | dds | 压缩的：WIC 自带；未压缩的：自己写的 |
| Photoshop | psd psb | oov/psd（8 位彩色，带透明）；16/32 位、索引色：自己写的合成图读取 |
| SVG | svg svgz | MyGo 的 SVG 渲染器 |
| 图元文件 | emf wmf | Windows 的 GDI 回放 |
| OpenEXR | exr | mrjoshuak/go-openexr（ZIP、PIZ、RLE 等） |
| Radiance HDR、PFM | hdr rgbe · pfm | mdouchement/hdr |
| TGA、QOI、PCX | tga icb vda vst · qoi · pcx | ftrvxmtrx/tga、xfmoulet/qoi、samuel/go-pcx |
| Netpbm | pbm pgm ppm pnm pam | spakin/netpbm |
| FITS | fits fit fts | astrogo/fitsio 读文件头，像素自己读（彩色、16 位、浮点） |
| 传真 | fax g3 | golang.org/x/image/ccitt |
| XBM、XPM、VIFF、WPG、CUT | xbm · xpm · viff xv · wpg · cut | 自己写的（没有 Go 库，每个几十行） |
| 相机 RAW | 3fr ari arw bay cap cr2 cr3 crw dcr dcs dng drf eip erf fff gpr iiq k25 kdc mdc mef mos mrw nef nrw orf ori pef ptx pxn r3d raf raw rw2 rwl rwz sr2 srf srw x3f | 浏览时：相机内嵌的 JPEG（够大时）；放大后：系统的 RAW 解码器；系统没有解码器的：只有内嵌的 JPEG，状态栏注明 |

HDR、EXR、PFM 里的光没有上限：不比白更亮的原样显示，有更亮部分的图整体压进屏幕的范围（Reinhard 曲线）。文件扩展名不对也没关系：按内容再认一遍。最后一档是系统的缩略图处理程序（别的程序给自己的文件类型装的），有就用，没有不影响上面任何一种。

**和 ImageGlass 10 的默认列表（81 个扩展名）比**：只有两个没有解码器。`flif`：没有 Go 库，格式已停止维护；ImageGlass 靠的 ImageMagick 在这台机器上的格式表里对它是既不能读也不能写。`obm`：查不到这是什么格式，ImageMagick 的格式表里也没有它。`mjpeg` 显示第一帧。多出来的有 jxr hdp wdp、jp2 j2k、tga 的别名、pam pfm、apng、avifs、fit fts、g3 等。

**怎么验证的**：`internal/codec` 的测试用同一张 97×61 的参考图让 ImageMagick 写出 36 种文件（各种压缩、位深），逐个解码后和参考图逐像素比，再把每个文件截断、改字节后解码，确认不会崩溃。ImageMagick 只用来在测试时造文件，程序本身不调用它；CI 的机器上没有它，这组测试会跳过。XBM、CUT 用手写的字节测。真实文件：4 个 HEIC（系统解码器和 gen2brain/heic 两条路都试了，后者慢 4 倍）、8 种相机的 RAW（CR2、CR3、NEF、ARW、DNG、RAF、ORF、RW2，来自 raw.pixls.us）都能显示。已知的不足：那张诺基亚 Lumia 1020 的 DNG，系统解码器只给出 880×656 的预览。

每种格式谁更快是量过的（[benchmark.md](benchmark.md#解码器)）：JPEG、PNG、WebP、LZW 的 TIFF 都是 WIC 更快，所以走 WIC。

**动图**：GIF、APNG 用 MyGo 的解码器逐帧合成；动态 WebP 用 gen2brain/webp（不依赖系统装没装 WebP 扩展）。解码在另一个线程上，提前两帧，五块缓冲轮流用。

**方向**：EXIF 方向在解码后转正（不重新编码文件）。旋转、翻转只改显示，不改文件。

**没做的**：色彩管理（ICC）目前交给解码器，不做显示器配置文件的转换；HDR 图像做了色调映射，不输出 HDR；不保存、不转换、不裁剪：MyImages 只看图。

## 界面

一个窗口：顶上一条 32 DIP 的栏既是标题栏又是工具栏（拖它移动窗口），中间是图，可选的缩略图条、信息面板、状态栏。全屏和幻灯片时只有图。

- 缩放以指针为中心，有 45 ms 的缓动；经过 100% 时停一下。放大后默认保持像素锐利。
- 「自动」模式：比窗口大的图缩到刚好放下，小的按原大小；矢量图（SVG、EMF）没有固有的像素大小，总是铺满窗口，并按窗口的像素重新画，所以放大也是锐利的。
- 菜单、对话框、设置都画在窗口里。
- 单实例：再打开一张图，交给已经在运行的窗口（设置里可以改成每张一个窗口）。

## 设为默认看图程序

Windows 不允许程序自己把自己设成默认（每个扩展名的选择带着系统算的校验值，只有「设置」能写），所以分两步：

1. 设置（Ctrl+,）里点「默认应用…」。程序先向系统登记自己能打开哪些类型（`shell.RegisterDefaults`：`HKCU\Software\MyImages\Capabilities` 和 `RegisteredApplications`，只写当前用户，每次写的内容一样），然后打开系统设置里 MyImages 的那一页（`ms-settings:defaultapps?registeredAppUser=MyImages`）。
2. 在那一页里点要交给 MyImages 的类型（.jpg、.png……），选 MyImages。

也试过让程序自己设（2026-11-12，Windows 11 26300）：把扩展名在 `HKCU\Software\Classes` 里指向自己，再删掉系统记的用户选择（`UserChoice`、`UserChoiceLatest`），这是老办法。结果系统立刻把选择重建成它自己的默认（.jpg 变成了「照片」），所以这段代码没有留下。伪造那个校验值等于绕过系统的保护，不做。

2026-11-12 在这台机器上试过：登记了 114 个扩展名，设置直接开在「应用 › 默认应用 › MyImages」，每个扩展名一行。也可以在任何图片上右键「打开方式 › 选择其他应用 › 始终」。

## 自动更新

用 MyGo 的 `mygo.Updater`（签名的更新包，发布在 GitHub Releases），界面是自己的（`app/update.go`）：

- 启动后立刻检查一次（另一个线程，不耽误启动），之后每 6 小时一次。
- 发现新版就在后台下载、验签、替换安装目录里的文件，期间什么都不显示；正在运行的程序不受影响。
- 换好之后工具栏上出现「重启以更新」。点一下，程序带着正在看的那张图重新启动，就是新版。
- 设置里的「自动检查更新」可以关，也可以手动点「检查更新」。离线、请求失败都不提示，等下一次。
- **密钥**：公钥写在 `mygo.json` 的 `updates.publicKey` 里，编进程序；私钥在本机的 `%APPDATA%\mygo\update-keys-myimages\mygo-update.key`，**不进仓库**。已安装的程序只接受这把私钥签过的更新：丢了就再也推不了更新，泄露了别人就能给用户推代码。发布工作流从仓库的 secret `MYGO_UPDATER_PRIVATE_KEY` 读它，没有就失败。
- 程序从仓库**最新的 release** 读 `update-windows-amd64.json`，所以仓库必须是公开的，更新才找得到。
- **从真正的 release 更新，2026-11-12 试过**：这台机器上用安装包装的 0.1.0，在 0.1.1 发布后正常启动（不带任何测试用的环境变量）。启动 2.1 秒后「重启以更新」出现（它从 GitHub 读到清单，下载了 22 KB 的增量包，验签，换好了文件）；点它，0.2 秒后同一张图出现在新进程里，`MyImages.exe --version` 是 0.1.1。
- 之后每个版本都这样更新过来：0.1.1 → 0.1.2（按钮 3.1 秒后出现，1.7 MB 的增量包；那次测试工具的点击被别的窗口挡住没点到）、0.1.2 → 0.1.3（按钮 2.3 秒后出现，点击后 0.2 秒新版带着同一张图回来）。
- 只有用安装包装的那份会更新（要能写自己的安装目录）；便携版和开发构建不会。
- 不发布也能试完整流程：构建一个旧版放到一个目录当作「已安装」，再构建一个版本号更高的，设 `MYIMAGES_UPDATE_DIR=<新版构建输出的目录>` 运行旧版，它从这个目录读清单和更新包，验签照常。2026-11-12 这样验证过：0.1.0 在 9 秒内把自己换成了 0.9.0，按钮出现在工具栏上。按钮也在真实窗口里点过（`tools/updatecheck`：等按钮出现，把鼠标点击发给那个窗口，再看程序是否回来）：点击后 0.2 秒，同一张图出现在新进程的窗口里，新进程是 0.9.0。

## 发布

- `ci.yml`：每次推送，测试并构建安装包，作为构建产物保存。
- `release.yml`：推一个 `v1.2.3` 标签，把版本号写进 `mygo.json`，构建安装包和签名的更新包（以及相对前几个版本的增量包），发布到 Releases：`myimages-setup-<版本>-amd64.exe`、便携版 exe、`update-windows-amd64.json`、`myimages-<版本>-windows-amd64.tar.gz`、`*.delta`。
- 安装包是 MyGo 用 NSIS 做的：装到 `%LOCALAPPDATA%\Programs\MyImages`，不需要管理员权限，注册「打开方式」里的图片类型（`mygo.json` 的 `fileAssociations`），不抢默认程序。

## 从 ImageGlass 学了什么

读了 ImageGlass 10 的源码（GPLv3；只取做法，没有搬代码）：

| | ImageGlass 10 | MyImages |
|---|---|---|
| 解码 | Skia 解常见格式，其余全交给 ImageMagick | 系统的 WIC 解常见格式，其余用 Go 的库 |
| 尺寸 | 总是全尺寸解码，另取一张预览先显示 | 先只解窗口大小，放大了才解全尺寸 |
| 预读 | 前后螺旋式预读，默认最多 10 张、1 GB | 前 3 后 1 的预览，每张几 MB |
| 快速翻页 | 按住方向键时只显示预览 | 一直只显示预览，所以不需要区分 |
| 大图 | 超过 8192 才分块 + mipmap | 所有图都分块上传，金字塔在解码线程上做 |
| 启动 | NativeAOT，后台初始化 Magick | 进程一开始就并行：显卡设备、解码、窗口 |
| 更新 | 每 7 天查一次，自动安装是付费功能 | 每次启动就查，自动下载，点一下重启 |

沿用的：螺旋式的预读顺序和按方向偏置；预读的取消；RAW 优先显示内嵌预览的思路；默认的鼠标和键盘习惯（滚轮缩放、双击切换缩放、方向键翻页）。

## MyGo 的补丁

`third_party/mygo` 是上游 0.9.0，改动在 `patches/mygo.patch`（`tools/setup-mygo.ps1` 取源码并打上）：

| 改动 | 用途 |
|---|---|
| `ui.Prewarm` | 程序一启动就在另一个线程上创建 Direct3D 设备 |
| 第一帧不等显卡 | 设备还没好时，窗口的帧先在内存里画，好了再换到 GPU。窗口提前约 100 ms 出现 |

## 测试

- `internal/app` 的测试用 MyGo 的 `ui.Tester` 在没有窗口的情况下跑整个界面：真的图片文件（临时目录里现画的 PNG、JPEG、GIF、SVG），真的解码，合成的按键、滚轮、拖动，然后读画出来的像素。覆盖：打开和翻页的顺序（自然排序、循环）、缩放（指针下的点不动）、拖动、只在放大后才解全尺寸、旋转、删除后到下一张、动图逐帧播放、矢量图铺满且边缘锐利、各面板和设置。
- 格式：见[格式](#格式)一节的「怎么验证的」。
- 真实窗口里验证过的：启动和切换的计时（`tools/startbench`）、截图（`--screenshot`）、更新（见上）。CI 构建的 0.1.0 安装包在这台机器上静默安装过（`/S`）：装到了 `%LOCALAPPDATA%\Programs\MyImages`，「应用」列表里有它，`.jpg` 的「打开方式」里有它，装好的那份启动和切换的计时与本地构建一致。`internal/shell` 的桌面测试（设 `MYIMAGES_DESKTOP_TESTS=1` 才跑，因为它们动真实的桌面）验证过：复制（剪贴板里有文件和位图两种格式）、删除到回收站、属性、「打开方式」、打印、在资源管理器中显示。设为壁纸在真实桌面上试过一次，功能是通的，但那次测试的「还原」把用户原来的壁纸弄丢了（系统的缓存文件被测试图覆盖），所以这个测试已删掉，不再碰真实桌面的壁纸。其余几项（各自弹出了系统的窗口：「属性」「选取应用」「打印图片」「文件资源管理器」，随后关掉）。
