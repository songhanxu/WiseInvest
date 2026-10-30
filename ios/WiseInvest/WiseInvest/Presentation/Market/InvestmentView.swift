import SwiftUI

private enum InvestmentMarket: String, CaseIterable, Identifiable {
    case all, aShare = "a_share", usStock = "us_stock", hkStock = "hk_stock", crypto
    var id: String { rawValue }
    var title: String {
        switch self {
        case .all: return "全部"
        case .aShare: return "A 股"
        case .usStock: return "美股"
        case .hkStock: return "港股"
        case .crypto: return "加密"
        }
    }
}

struct InvestmentView: View {
    @State private var tab = 0
    @State private var marketFilter = "all"
    @State private var stocks: [InvestmentStock] = []
    @State private var query = ""
    @State private var results: [InvestmentStock] = []
    @State private var accounts: [InvestmentAccount] = []
    @State private var snapshot: InvestmentSnapshot?
    @State private var alertState: InvestmentAlertResponse?
    @State private var email = ""
    @State private var holdingTargets: [String: String] = [:]
    @State private var alertedHoldings: Set<String> = []
    @State private var loading = false
    @State private var error: String?
    @State private var accountSyncError: String?
    @State private var didRequestInsights = false
    private let service = InvestmentDataService.shared

    private var filteredStocks: [InvestmentStock] {
        marketFilter == "all" ? stocks : stocks.filter { $0.market == marketFilter }
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    VStack(alignment: .leading, spacing: 5) {
                        Text("投资工作台").font(.caption).foregroundStyle(.secondary)
                        Text("我的投资").font(.system(size: 31, weight: .semibold))
                        Text("关注的公司、最新财报与账户持仓").font(.subheadline).foregroundStyle(.secondary)
                    }
                    .padding(.top, 14)

                    Picker("内容", selection: $tab) {
                        Text("自选研究").tag(0)
                        Text("账户持仓").tag(1)
                    }
                    .pickerStyle(.segmented)

                    if tab == 0 { watchlistContent } else { accountsContent }
                }
                .padding(.horizontal, 18)
                .padding(.bottom, 30)
            }
            .background(Color.primaryBackground)
            .navigationBarTitleDisplayMode(.inline)
            .refreshable { await load() }
            .task(id: tab) { await load() }
            .task(id: query) { await search() }
        }
    }

    private var watchlistContent: some View {
        VStack(alignment: .leading, spacing: 16) {
            if let error {
                issueNotice("请求暂未完成", details: error)
            }
            VStack(alignment: .leading, spacing: 12) {
                Text("添加自选").font(.headline)
                HStack {
                    Image(systemName: "magnifyingglass").foregroundStyle(.secondary)
                    TextField("搜索名称或代码", text: $query)
                        .textInputAutocapitalization(.characters)
                        .autocorrectionDisabled()
                }
                .padding(11)
                .background(Color.primaryBackground, in: RoundedRectangle(cornerRadius: 9))
                if !results.isEmpty {
                    ForEach(results) { stock in
                        HStack {
                            VStack(alignment: .leading) {
                                Text(stock.name).font(.subheadline.weight(.medium))
                                Text("\(stock.symbol) · \(stock.market)").font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer()
                            Button("添加") { Task { await add(stock) } }
                                .font(.subheadline)
                        }
                        .padding(.vertical, 6)
                    }
                }
            }
            .investmentCard()

            Text("我的自选")
                .font(.headline)
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    filterButton("全部", id: "all")
                    ForEach(InvestmentMarket.allCases.filter { $0 != .all }) { market in filterButton(market.title, id: market.id) }
                }
            }

            if loading && stocks.isEmpty { ProgressView("正在更新数据…").frame(maxWidth: .infinity).padding(30) }
            else if filteredStocks.isEmpty { Text("暂无自选标的，搜索后添加。").foregroundStyle(.secondary).frame(maxWidth: .infinity).investmentCard() }

            ForEach(filteredStocks) { stock in
                NavigationLink(value: stock) {
                    HStack(alignment: .top) {
                        VStack(alignment: .leading, spacing: 7) {
                            Text(stock.name).font(.headline).foregroundStyle(.primary)
                            Text("\(stock.symbol) · \(stock.market)").font(.caption).foregroundStyle(.secondary)
                            HStack(spacing: 8) {
                                Text(stock.currentPrice.map { String(format: "%.2f", $0) } ?? "—")
                                    .font(.title3.weight(.semibold)).foregroundStyle(.primary)
                                Text(stock.currency ?? "").font(.caption).foregroundStyle(.secondary)
                                if let change = stock.changePercent {
                                    Text(String(format: "%+.2f%%", change)).font(.caption)
                                        .foregroundStyle(change >= 0 ? .red : .green)
                                }
                            }
                        }
                        Spacer()
                        Button { Task { await remove(stock) } } label: {
                            Image(systemName: "trash").font(.subheadline).foregroundStyle(.secondary)
                        }
                        .buttonStyle(.plain)
                    }
                    .investmentCard()
                }
                .buttonStyle(.plain)
            }
        }
        .navigationDestination(for: InvestmentStock.self) { stock in
            InvestmentResearchView(stock: stock)
        }
    }

    private func filterButton(_ title: String, id: String) -> some View {
        Button(title) { marketFilter = id }
            .font(.subheadline)
            .foregroundStyle(marketFilter == id ? Color.white : Color.textPrimary)
            .padding(.horizontal, 14).padding(.vertical, 8)
            .background(marketFilter == id ? Color.accentBlue : Color.secondaryBackground, in: Capsule())
    }

    private var accountsContent: some View {
        VStack(alignment: .leading, spacing: 15) {
            if let details = accountNoticeDetails {
                issueNotice(accountSyncError == nil ? "部分持仓数据尚未更新" : "账户同步暂不可用", details: details)
            }
            if let error {
                issueNotice("请求暂未完成", details: error)
            }
            if loading && accounts.isEmpty && snapshot == nil {
                ProgressView("正在读取账户…")
                    .frame(maxWidth: .infinity)
                    .padding(20)
            }
            if let snapshot {
                ForEach(snapshot.accountEquities ?? []) { equity in
                    VStack(alignment: .leading, spacing: 6) {
                        Text("\(equity.accountID) · 账户资产").font(.caption).foregroundStyle(.secondary)
                        Text("\(equity.amount) \(equity.currency)").font(.title3.weight(.semibold))
                    }.frame(maxWidth: .infinity, alignment: .leading).investmentCard()
                }
            }
            ForEach(accounts) { account in
                VStack(alignment: .leading, spacing: 5) {
                    Text(account.name).font(.headline)
                    Text(account.status == "ready" ? "已配置 · 只读" : account.message ?? account.status)
                        .font(.caption).foregroundStyle(.secondary)
                }.frame(maxWidth: .infinity, alignment: .leading).investmentCard()
            }
            if let snapshot, snapshot.holdings.isEmpty {
                Text("暂无可展示的持仓").foregroundStyle(.secondary).frame(maxWidth: .infinity).investmentCard()
            }
            ForEach(snapshot?.holdings ?? []) { holding in
                VStack(alignment: .leading, spacing: 7) {
                    Text(holding.name ?? holding.symbol ?? "持仓").font(.headline)
                    Text("\(holding.accountName ?? "账户") · \(holding.quantity) \(holding.quantityUnit ?? "")")
                        .font(.caption).foregroundStyle(.secondary)
                    HStack {
                        Text("\(holding.marketValue ?? "—") \(holding.currency ?? "")").font(.subheadline.weight(.medium))
                        Spacer()
                        Text("盈亏 \(holding.unrealizedPnL ?? "—")").font(.caption).foregroundStyle(.secondary)
                    }
                    HStack {
                        TextField("目标价格", text: Binding(
                            get: { holdingTargets[holding.holdingID] ?? "" },
                            set: { holdingTargets[holding.holdingID] = $0 }
                        ))
                        .keyboardType(.decimalPad)
                        .padding(9)
                        .background(Color.primaryBackground, in: RoundedRectangle(cornerRadius: 8))
                        Button("保存提醒") { Task { await saveHoldingAlert(holding) } }
                        if alertedHoldings.contains(holding.holdingID) {
                            Button("删除") { Task { await removeHoldingAlert(holding) } }
                        }
                    }.font(.caption)
                }.investmentCard()
            }
            VStack(alignment: .leading, spacing: 9) {
                Text("价格提醒邮箱").font(.headline)
                TextField("接收提醒的邮箱", text: $email).textContentType(.emailAddress)
                    .keyboardType(.emailAddress).textInputAutocapitalization(.never)
                    .padding(10).background(Color.primaryBackground, in: RoundedRectangle(cornerRadius: 8))
                Button("保存邮箱") { Task { await saveEmail() } }.font(.subheadline)
                if alertState?.mailerConfigured == false {
                    Text("邮件服务尚未配置，提醒暂不会发送。")
                        .font(.caption).foregroundStyle(.secondary)
                }
            }.frame(maxWidth: .infinity, alignment: .leading).investmentCard()
        }
    }

    private var accountNoticeDetails: String? {
        var messages: [String] = []
        if let accountSyncError { messages.append(accountSyncError) }
        if accountSyncError != nil && snapshot != nil {
            messages.append("当前展示的是上次取得的持仓，向下拉动可重试同步。")
        }
        if snapshot?.stale == true { messages.append("当前展示的是上次取得的持仓，向下拉动可重试同步。") }
        messages.append(contentsOf: snapshot?.warnings ?? [])
        var seen: Set<String> = []
        let unique = messages.filter { seen.insert($0).inserted }
        return unique.isEmpty ? nil : unique.joined(separator: "\n\n")
    }

    private func issueNotice(_ title: String, details: String) -> some View {
        DisclosureGroup {
            Text(details)
                .font(.caption)
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.top, 8)
        } label: {
            Label(title, systemImage: "exclamationmark.circle")
                .font(.subheadline.weight(.medium))
                .foregroundStyle(Color.textPrimary)
        }
        .tint(Color.textSecondary)
        .padding(.horizontal, 13)
        .padding(.vertical, 11)
        .background(Color.secondaryBackground, in: RoundedRectangle(cornerRadius: 12))
    }

    @MainActor private func load() async {
        let requestedTab = tab
        loading = true
        defer { if tab == requestedTab { loading = false } }
        error = nil
        if requestedTab == 0 {
            do {
                let loaded = try await service.watchlist()
                guard !Task.isCancelled && tab == requestedTab else { return }
                stocks = loaded
                if !didRequestInsights && stocks.contains(where: { $0.market == "us_stock" }) {
                    didRequestInsights = true
                    Task { try? await service.refreshAllInsights() }
                }
            } catch {
                if !Task.isCancelled && tab == requestedTab { self.error = error.localizedDescription }
            }
            return
        }
        accountSyncError = nil
        guard AuthState.shared.isAuthenticated else { return }
        var syncErrors: [String] = []
        do { accounts = try await service.accounts() }
        catch { if !Task.isCancelled { syncErrors.append(error.localizedDescription) } }
        guard !Task.isCancelled && tab == requestedTab else { return }
        guard AuthState.shared.isAuthenticated else { accountSyncError = syncErrors.first; return }
        do { snapshot = try await service.holdings() }
        catch { if !Task.isCancelled { syncErrors.append(error.localizedDescription) } }
        guard !Task.isCancelled && tab == requestedTab else { return }
        accountSyncError = syncErrors.isEmpty ? nil : syncErrors.joined(separator: "\n\n")
        guard AuthState.shared.isAuthenticated else { return }
        do {
            let loaded = try await service.alerts()
            guard !Task.isCancelled && tab == requestedTab else { return }
            alertState = loaded
            email = alertState?.email ?? ""
            alertedHoldings = Set(alertState?.alerts.map(\.holdingID) ?? [])
            holdingTargets = Dictionary(uniqueKeysWithValues: alertState?.alerts.map { ($0.holdingID, String($0.targetPrice)) } ?? [])
        } catch {
            if !Task.isCancelled && tab == requestedTab { self.error = error.localizedDescription }
        }
    }

    @MainActor private func search() async {
        let value = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !value.isEmpty else { results = []; return }
        try? await Task.sleep(nanoseconds: 300_000_000)
        guard !Task.isCancelled else { return }
        do { results = try await service.search(value, market: InvestmentMarket.all.rawValue) }
        catch { if !Task.isCancelled { self.error = error.localizedDescription } }
    }

    @MainActor private func add(_ stock: InvestmentStock) async {
        do { try await service.add(stock); query = ""; results = []; stocks = try await service.watchlist() }
        catch { self.error = error.localizedDescription }
    }
    @MainActor private func remove(_ stock: InvestmentStock) async {
        do { try await service.remove(stock); stocks = try await service.watchlist() }
        catch { self.error = error.localizedDescription }
    }
    @MainActor private func saveEmail() async {
        do { try await service.saveEmail(email.trimmingCharacters(in: .whitespacesAndNewlines)); alertState = try await service.alerts() }
        catch { self.error = error.localizedDescription }
    }
    @MainActor private func saveHoldingAlert(_ holding: InvestmentHolding) async {
        guard let price = Double(holdingTargets[holding.holdingID] ?? ""), price.isFinite, price > 0 else {
            error = "请输入有效的目标价格"; return
        }
        do {
            try await service.saveAlert(holdingID: holding.holdingID, targetPrice: price, accountID: holding.accountID)
            alertedHoldings.insert(holding.holdingID); error = nil
        } catch { self.error = error.localizedDescription }
    }
    @MainActor private func removeHoldingAlert(_ holding: InvestmentHolding) async {
        do {
            try await service.removeAlert(holdingID: holding.holdingID)
            alertedHoldings.remove(holding.holdingID); holdingTargets[holding.holdingID] = ""
            error = nil
        } catch { self.error = error.localizedDescription }
    }
}

private struct InvestmentCardModifier: ViewModifier {
    func body(content: Content) -> some View {
        content.padding(16)
            .foregroundStyle(Color.textPrimary)
            .background(Color.secondaryBackground, in: RoundedRectangle(cornerRadius: 12))
    }
}
private extension View {
    func investmentCard() -> some View { modifier(InvestmentCardModifier()) }
}
