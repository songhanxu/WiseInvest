import SwiftUI

struct InvestmentResearchView: View {
    let stock: InvestmentStock
    @State private var fundamentals: InvestmentFundamentals?
    @State private var insight: InvestmentInsight?
    @State private var targetPrice = ""
    @State private var hasAlert = false
    @State private var loading = true
    @State private var error: String?
    private let service = InvestmentDataService.shared

    private var holdingID: String { "watchlist:\(stock.market):\(stock.id)" }
    private var latestReport: InvestmentReport? { fundamentals?.financialReports?.first }
    private var insightIsCurrent: Bool {
        insight?.status == "ready" && insight?.accession == latestReport?.accession
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 17) {
                VStack(alignment: .leading, spacing: 5) {
                    Text("\(stock.market) · \(stock.symbol)")
                        .font(.caption).foregroundStyle(.secondary)
                    Text(stock.name).font(.system(size: 30, weight: .semibold))
                    Text("财务数据与最新申报").font(.subheadline).foregroundStyle(.secondary)
                }.padding(.top, 14)

                if let error { Text(error).font(.footnote).foregroundStyle(.orange) }
                if loading { ProgressView("正在读取财务数据…").frame(maxWidth: .infinity).padding(30) }

                if let fundamentals {
                    VStack(alignment: .leading, spacing: 14) {
                        Text("财务概览").font(.headline)
                        HStack {
                            metric("每股收益 EPS", value: fundamentals.eps)
                            Spacer()
                            metric("市盈率 PE", value: fundamentals.pe)
                        }
                        HStack {
                            metric("预期 PE", value: fundamentals.forwardPE)
                            Spacer()
                            VStack(alignment: .leading, spacing: 4) {
                                Text("下次财报").font(.caption).foregroundStyle(.secondary)
                                Text(fundamentals.nextEarningsDate ?? "待确认")
                                    .font(.subheadline.weight(.medium))
                            }
                        }
                        if let message = fundamentals.message, !message.isEmpty {
                            Text(message).font(.caption).foregroundStyle(.secondary)
                        }
                    }.researchCard()
                }

                if stock.market == "us_stock" {
                    VStack(alignment: .leading, spacing: 12) {
                        Text("最新财报 AI 解读").font(.headline)
                        if insightIsCurrent, let insight {
                            Text("报告期末 \(insight.reportDate ?? "—") · 生成于 \(insight.generatedAt ?? "—")")
                                .font(.caption).foregroundStyle(.secondary)
                            MarkdownContentView(content: insight.content ?? "")
                            if let sources = insight.sources, !sources.isEmpty {
                                Text("核对来源").font(.subheadline.weight(.medium))
                                ForEach(sources) { source in
                                    if let url = URL(string: source.url), url.scheme == "https" {
                                        Link(source.label, destination: url).font(.subheadline)
                                    }
                                }
                            }
                        } else if insight?.status == "pending" {
                            HStack { ProgressView(); Text("正在核对财报并生成解读…") }
                                .font(.subheadline).foregroundStyle(.secondary)
                        } else {
                            Text(latestReport == nil ? "暂无可核验的 SEC 财报" : insight?.message ?? "今天尚无可用的财报解读")
                                .font(.subheadline).foregroundStyle(.secondary)
                            if latestReport != nil {
                                Button(insight?.status == "failed" || insight?.status == "ready" ? "重新生成" : "开始生成") {
                                    Task { await refreshInsight(retry: true) }
                                }
                            }
                        }
                    }.researchCard()
                }

                VStack(alignment: .leading, spacing: 14) {
                    Text("财报原文与指标").font(.headline)
                    if fundamentals?.financialReports?.isEmpty != false {
                        Text("暂无可核验的财报").font(.subheadline).foregroundStyle(.secondary)
                    }
                    ForEach(fundamentals?.financialReports ?? []) { report in
                        VStack(alignment: .leading, spacing: 8) {
                            Text("\(report.displayForm) · \(report.reportDate)").font(.subheadline.weight(.semibold))
                            Text("提交 SEC \(report.filedDate)").font(.caption).foregroundStyle(.secondary)
                            if report.kind == "earnings_release" {
                                Text("业绩发布稿；完整 10-Q/10-K 尚未提交。")
                                    .font(.caption).foregroundStyle(.secondary)
                            }
                            ForEach((report.metrics ?? [:]).keys.sorted(), id: \.self) { key in
                                if let metric = report.metrics?[key] {
                                    HStack(alignment: .top) {
                                        Text(Self.metricLabel(key)).foregroundStyle(.secondary)
                                        Spacer()
                                        Text("\(metric.value.formatted()) \(metric.unit)")
                                    }.font(.caption)
                                }
                            }
                            if let url = URL(string: report.sourceURL), url.scheme == "https" {
                                Link("查看 SEC 财报原文 ↗", destination: url).font(.subheadline)
                            }
                        }
                        .padding(.vertical, 8)
                    }
                }.researchCard()

                VStack(alignment: .leading, spacing: 12) {
                    Text("价格提醒").font(.headline)
                    Text("达到目标价后发送邮件。接收邮箱可在投资页设置。")
                        .font(.caption).foregroundStyle(.secondary)
                    HStack {
                        TextField("目标价格", text: $targetPrice).keyboardType(.decimalPad)
                            .padding(10).background(Color.primaryBackground, in: RoundedRectangle(cornerRadius: 8))
                        Button("保存") { Task { await saveAlert() } }
                        if hasAlert { Button("删除") { Task { await removeAlert() } } }
                    }.font(.subheadline)
                }.researchCard()
            }
            .padding(.horizontal, 18).padding(.bottom, 30)
        }
        .background(Color.primaryBackground)
        .navigationTitle("公司研究")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await load() }
        .task { await load() }
        .task(id: insight?.status) {
            guard insight?.status == "pending" else { return }
            while !Task.isCancelled && insight?.status == "pending" {
                try? await Task.sleep(nanoseconds: 5_000_000_000)
                if !Task.isCancelled { await fetchInsight() }
            }
        }
    }

    private func metric(_ title: String, value: Double?) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title).font(.caption).foregroundStyle(.secondary)
            Text(value.map { $0.formatted() } ?? "—").font(.title3.weight(.semibold))
        }.frame(maxWidth: .infinity, alignment: .leading)
    }

    private static func metricLabel(_ key: String) -> String {
        ["revenue": "营业收入", "net_income": "净利润", "assets": "总资产", "liabilities": "总负债",
         "equity": "股东权益", "operating_cash_flow": "经营现金流" ][key] ?? key.replacingOccurrences(of: "_", with: " ")
    }

    @MainActor private func load() async {
        loading = true; error = nil
        do { fundamentals = try await service.fundamentals(stock) }
        catch { self.error = error.localizedDescription }
        do {
            let alert = try await service.alerts().alerts.first { $0.holdingID == holdingID }
            hasAlert = alert != nil
            targetPrice = alert.map { String($0.targetPrice) } ?? ""
        } catch { self.error = error.localizedDescription }
        if stock.market == "us_stock" && latestReport != nil {
            await refreshInsight(retry: false)
        }
        loading = false
    }

    @MainActor private func fetchInsight() async {
        do { insight = try await service.insights()[stock.id] }
        catch { self.error = error.localizedDescription }
    }

    @MainActor private func refreshInsight(retry: Bool) async {
        do {
            try await service.refreshInsight(code: stock.id, retry: retry, accession: retry ? latestReport?.accession : nil)
            await fetchInsight()
        } catch { self.error = error.localizedDescription }
    }

    @MainActor private func saveAlert() async {
        guard let price = Double(targetPrice), price.isFinite, price > 0 else {
            error = "请输入有效的目标价格"; return
        }
        do { try await service.saveAlert(holdingID: holdingID, targetPrice: price); hasAlert = true; error = nil }
        catch { self.error = error.localizedDescription }
    }
    @MainActor private func removeAlert() async {
        do { try await service.removeAlert(holdingID: holdingID); hasAlert = false; targetPrice = "" }
        catch { self.error = error.localizedDescription }
    }
}

private struct InvestmentResearchCard: ViewModifier {
    func body(content: Content) -> some View {
        content.frame(maxWidth: .infinity, alignment: .leading)
            .padding(16)
            .foregroundStyle(Color.textPrimary)
            .background(Color.secondaryBackground, in: RoundedRectangle(cornerRadius: 12))
    }
}
private extension View {
    func researchCard() -> some View { modifier(InvestmentResearchCard()) }
}
