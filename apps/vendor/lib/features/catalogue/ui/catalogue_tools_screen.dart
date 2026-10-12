import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../settings/ui/settings_design.dart';
import '../state/catalogue_tools_controller.dart';
import '../state/catalogue_resource.dart';
import '../state/advanced_vocabulary.dart';
import 'catalogue_screen.dart';

class CataloguePriceScreen extends StatefulWidget {
  const CataloguePriceScreen({
    super.key,
    required this.runtime,
    required this.record,
    required this.food,
  });
  final VendorRuntime runtime;
  final JsonObject record;
  final bool food;
  @override
  State<CataloguePriceScreen> createState() => _CataloguePriceScreenState();
}

class _CataloguePriceScreenState extends State<CataloguePriceScreen> {
  late final c = CatalogueToolsController(
    widget.runtime.backend,
    widget.food ? CatalogueTool.foodPrice : CatalogueTool.shopPrice,
    record: widget.record,
  );
  final quantity = TextEditingController(text: '1');
  @override
  void initState() {
    super.initState();
    c.prepare();
  }

  @override
  void dispose() {
    quantity.dispose();
    c.dispose();
    super.dispose();
  }

  bool get validQuantity {
    final n = int.tryParse(quantity.text.trim());
    return n != null && n >= 1 && n <= 1000000000;
  }

  Future<void> preview() async {
    FocusManager.instance.primaryFocus?.unfocus();
    await c.preview(
      quantity: widget.food ? 1 : int.parse(quantity.text.trim()),
    );
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: const Text('Price preview'),
        actions: [
          IconButton(
            tooltip: 'Refresh choices',
            onPressed: c.loading || c.preparing ? null : c.prepare,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      bottomNavigationBar: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 12, 24, 16),
          child: SettingsAction(
            'Check price',
            icon: Icons.calculate_outlined,
            busy: c.loading,
            onPressed:
                c.ready &&
                    !c.preparing &&
                    !c.loading &&
                    (widget.food ? c.selectionValid : validQuantity)
                ? preview
                : null,
          ),
        ),
      ),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(24, 10, 24, 28),
        children: [
          _ToolHeading(
            icon: widget.food
                ? Icons.restaurant_menu_rounded
                : Icons.price_check_rounded,
            title: widget.food
                ? (c.subject['name'] ?? widget.record['name'] ?? 'Food item')
                      .toString()
                : 'Check a selling price',
            subtitle: widget.food
                ? 'Choose a size and extras.'
                : 'Choose the quantity, unit and sales channel.',
          ),
          if (c.preparing)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 30),
              child: PremiumSkeleton(rows: 2),
            ),
          if (c.error != null) SettingsNote(c.error!, warning: c.restricted),
          if (!c.ready && !c.preparing)
            SettingsAction(
              'Reload choices',
              secondary: true,
              icon: Icons.refresh_rounded,
              onPressed: c.prepare,
            ),
          if (c.ready) ...[
            if (widget.food) ...[
              if (c.variants.isNotEmpty)
                SettingsCard(
                  child: DropdownButtonFormField<String>(
                    initialValue: c.variantId ?? '',
                    isExpanded: true,
                    decoration: const InputDecoration(labelText: 'Variant'),
                    items: [
                      const DropdownMenuItem(
                        value: '',
                        child: Text('Base item'),
                      ),
                      for (final v in c.variants)
                        DropdownMenuItem(
                          value: v['id'].toString(),
                          child: Text(
                            v['name'].toString(),
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                    ],
                    onChanged: c.loading
                        ? null
                        : (v) => c.selectVariant(v == '' ? null : v),
                  ),
                ),
              for (final group in c.groups)
                Padding(
                  padding: const EdgeInsets.only(top: 16),
                  child: SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        SettingsHeading(
                          group['name'].toString(),
                          subtitle:
                              'Choose ${group['min_select']} to ${group['max_select']}',
                        ),
                        for (final m in group['modifiers'] as List)
                          CheckboxListTile(
                            contentPadding: EdgeInsets.zero,
                            controlAffinity: ListTileControlAffinity.leading,
                            activeColor: KarryamColors.tealInk.forInk(context),
                            title: Text(m['name'].toString()),
                            value: c.selectedModifiers.contains(m['id']),
                            onChanged:
                                c.loading ||
                                    !c.selectedModifiers.contains(m['id']) &&
                                        (group['modifiers'] as List)
                                                .where(
                                                  (x) => c.selectedModifiers
                                                      .contains(x['id']),
                                                )
                                                .length >=
                                            group['max_select']
                                ? null
                                : (selected) => c.toggleModifier(
                                    m['id'].toString(),
                                    selected == true,
                                  ),
                          ),
                        if ((group['modifiers'] as List).isEmpty)
                          const Text('No available choices in this group.'),
                      ],
                    ),
                  ),
                ),
            ] else
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Wrap(
                      spacing: 10,
                      runSpacing: 10,
                      children: [
                        for (final channel in ['retail', 'wholesale'])
                          ChoiceChip(
                            label: Text(dashboardLabel(channel)),
                            selected: c.channel == channel,
                            selectedColor: KarryamColors.mint.forSurface(
                              context,
                            ),
                            labelStyle: TextStyle(
                              color:
                                  (c.channel == channel
                                          ? KarryamColors.tealInk
                                          : KarryamColors.indigo)
                                      .forInk(context),
                              fontWeight: FontWeight.w600,
                            ),
                            side: BorderSide(
                              color: Color(0xFFDDE7E3).forOutline(context),
                            ),
                            onSelected: c.loading
                                ? null
                                : (_) => c.selectChannel(channel),
                          ),
                      ],
                    ),
                    const SizedBox(height: 22),
                    DropdownButtonFormField<String>(
                      initialValue: c.sellUnitId ?? '',
                      isExpanded: true,
                      decoration: const InputDecoration(labelText: 'Sell unit'),
                      items: [
                        const DropdownMenuItem(
                          value: '',
                          child: Text('Variant unit'),
                        ),
                        for (final u in c.units)
                          DropdownMenuItem(
                            value: u['id'].toString(),
                            child: Text(
                              u['display_name'].toString(),
                              maxLines: 2,
                              overflow: TextOverflow.ellipsis,
                            ),
                          ),
                      ],
                      onChanged: c.loading
                          ? null
                          : (v) => c.selectUnit(v == '' ? null : v),
                    ),
                    const SizedBox(height: 18),
                    TextField(
                      controller: quantity,
                      enabled: !c.loading,
                      keyboardType: TextInputType.number,
                      decoration: InputDecoration(
                        labelText: 'Quantity',
                        errorText: validQuantity
                            ? null
                            : 'Enter 1 to 1,000,000,000',
                      ),
                      onChanged: (_) => c.invalidate(),
                    ),
                  ],
                ),
              ),
          ],
          const SizedBox(height: 22),
          AnimatedSwitcher(
            duration: const Duration(milliseconds: 260),
            transitionBuilder: (child, animation) => FadeTransition(
              opacity: animation,
              child: SlideTransition(
                position: Tween(
                  begin: const Offset(0, .04),
                  end: Offset.zero,
                ).animate(animation),
                child: child,
              ),
            ),
            child: c.result.isNotEmpty
                ? _PriceResult(
                    key: ValueKey(c.result),
                    data: c.result,
                    food: widget.food,
                  )
                : !c.preparing && c.ready
                ? _ToolPrompt(
                    key: const ValueKey('prompt'),
                    icon: Icons.receipt_long_outlined,
                    title: 'Your price breakdown',
                    message: widget.food
                        ? 'Complete your choices, then check the price.'
                        : 'Check the price to see the total and order rules.',
                  )
                : const SizedBox.shrink(),
          ),
        ],
      ),
    ),
  );
}

class CatalogueLookupScreen extends StatefulWidget {
  const CatalogueLookupScreen({
    super.key,
    required this.runtime,
    required this.currency,
    this.writable = true,
  });
  final VendorRuntime runtime;
  final String currency;
  final bool writable;
  @override
  State<CatalogueLookupScreen> createState() => _CatalogueLookupScreenState();
}

class _CatalogueLookupScreenState extends State<CatalogueLookupScreen> {
  late final c = CatalogueToolsController(
    widget.runtime.backend,
    CatalogueTool.lookup,
  );
  late final input = TextEditingController(
    text: widget.runtime.mode == VendorDataMode.mock ? 'DEMO-PACK-01' : '',
  );
  String? type;
  @override
  void dispose() {
    input.dispose();
    c.dispose();
    super.dispose();
  }

  Future<void> search() async {
    FocusManager.instance.primaryFocus?.unfocus();
    await c.lookup(input.text, type: type);
  }

  Future<void> open(bool variant) async {
    final data = jsonObject(c.result[variant ? 'variant' : 'product']);
    await Navigator.push(
      context,
      setupRoute<void>(
        CatalogueDetailScreen(
          runtime: widget.runtime,
          resource: CatalogueResource(
            variant ? CatalogueKind.shopVariants : CatalogueKind.products,
            parameters: variant
                ? {
                    'productId': jsonObject(
                      c.result['product'],
                    )['id'].toString(),
                  }
                : const {},
            writable: widget.writable,
          ),
          row: data,
          currency: widget.currency,
        ),
      ),
    );
    if (mounted) c.invalidate();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(title: const Text('Find a product')),
      bottomNavigationBar: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 12, 24, 16),
          child: SettingsAction(
            'Find product',
            icon: Icons.search_rounded,
            busy: c.loading,
            onPressed: input.text.trim().isEmpty || c.loading ? null : search,
          ),
        ),
      ),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(24, 10, 24, 28),
        children: [
          const _ToolHeading(
            icon: Icons.barcode_reader,
            title: 'Find it by its code',
            subtitle: 'Search your catalogue using a SKU or barcode.',
          ),
          SettingsCard(
            child: Column(
              children: [
                TextField(
                  controller: input,
                  enabled: !c.loading,
                  textInputAction: TextInputAction.search,
                  autocorrect: false,
                  enableSuggestions: false,
                  decoration: InputDecoration(
                    labelText: 'SKU or barcode',
                    prefixIcon: Icon(
                      Icons.qr_code_2_rounded,
                      color: KarryamColors.tealInk.forInk(context),
                    ),
                    suffixIcon: input.text.isEmpty
                        ? null
                        : IconButton(
                            tooltip: 'Clear code',
                            onPressed: c.loading
                                ? null
                                : () {
                                    input.clear();
                                    c.invalidate();
                                  },
                            icon: const Icon(Icons.close_rounded),
                          ),
                  ),
                  onChanged: (_) => c.invalidate(),
                  onSubmitted: (_) => search(),
                ),
                const SizedBox(height: 20),
                DropdownButtonFormField<String>(
                  initialValue: type ?? '',
                  isExpanded: true,
                  decoration: const InputDecoration(
                    labelText: 'Identifier type',
                  ),
                  items: [
                    const DropdownMenuItem(value: '', child: Text('All types')),
                    for (final t in catalogueVocabularyList('IdentifierTypes'))
                      DropdownMenuItem(
                        value: t,
                        child: Text(
                          dashboardLabel(t),
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                  ],
                  onChanged: c.loading
                      ? null
                      : (v) {
                          type = v == '' ? null : v;
                          c.invalidate();
                        },
                ),
              ],
            ),
          ),
          const SizedBox(height: 22),
          if (c.error != null) SettingsNote(c.error!, warning: c.restricted),
          AnimatedSwitcher(
            duration: const Duration(milliseconds: 240),
            child: c.result.isNotEmpty
                ? SettingsCard(
                    key: ValueKey(c.result),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          children: [
                            Icon(
                              Icons.inventory_2_outlined,
                              color: KarryamColors.tealInk.forInk(context),
                              size: 32,
                            ),
                            const Spacer(),
                            SettingsStatus(
                              jsonObject(c.result['product'])['status'],
                            ),
                          ],
                        ),
                        const SizedBox(height: 20),
                        SettingsHeading(
                          jsonObject(c.result['product'])['name'].toString(),
                          subtitle: jsonObject(
                            c.result['variant'],
                          )['name'].toString(),
                        ),
                        SettingsFact(
                          'Code',
                          jsonObject(
                            c.result['identifier'],
                          )['identifier_value'],
                        ),
                        SettingsFact(
                          'Type',
                          dashboardLabel(
                            jsonObject(
                              c.result['identifier'],
                            )['identifier_type'],
                          ),
                        ),
                        if (jsonObject(c.result['variant'])['sku'] != null)
                          SettingsFact(
                            'SKU',
                            jsonObject(c.result['variant'])['sku'],
                          ),
                        SettingsFact(
                          'Variant status',
                          dashboardLabel(
                            jsonObject(c.result['variant'])['status'],
                          ),
                        ),
                        SettingsFact(
                          'External verification',
                          jsonObject(
                                    c.result['identifier'],
                                  )['externally_verified'] ==
                                  true
                              ? 'Verified'
                              : 'Not verified',
                        ),
                        const SizedBox(height: 20),
                        SettingsAction(
                          'Open product',
                          icon: Icons.shopping_bag_outlined,
                          onPressed: () => open(false),
                        ),
                        const SizedBox(height: 12),
                        SettingsAction(
                          'Open variant',
                          secondary: true,
                          icon: Icons.layers_outlined,
                          onPressed: () => open(true),
                        ),
                      ],
                    ),
                  )
                : c.notFound
                ? const _ToolPrompt(
                    key: ValueKey('missing'),
                    icon: Icons.manage_search_rounded,
                    title: 'No matching product',
                    message: 'Check the code or try another identifier type.',
                  )
                : c.error == null
                ? const _ToolPrompt(
                    key: ValueKey('ready'),
                    icon: Icons.barcode_reader,
                    title: 'A direct route to your product',
                    message:
                        'Enter an identifier to open its product and variant.',
                  )
                : const SizedBox.shrink(),
          ),
        ],
      ),
    ),
  );
}

class _ToolHeading extends StatelessWidget {
  const _ToolHeading({
    required this.icon,
    required this.title,
    required this.subtitle,
  });
  final IconData icon;
  final String title, subtitle;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(bottom: 22),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 64,
          height: 64,
          decoration: BoxDecoration(
            color: KarryamColors.mint.forSurface(context),
            borderRadius: BorderRadius.circular(22),
          ),
          child: Icon(
            icon,
            color: KarryamColors.tealInk.forInk(context),
            size: 32,
          ),
        ),
        const SizedBox(height: 20),
        SettingsHeading(title, subtitle: subtitle),
      ],
    ),
  );
}

class _ToolPrompt extends StatelessWidget {
  const _ToolPrompt({
    super.key,
    required this.icon,
    required this.title,
    required this.message,
  });
  final IconData icon;
  final String title, message;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 22, horizontal: 12),
    child: Column(
      children: [
        Icon(icon, size: 54, color: Color(0xFF769BA0).forInk(context)),
        const SizedBox(height: 16),
        Text(
          title,
          textAlign: TextAlign.center,
          style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 19),
        ),
        const SizedBox(height: 8),
        Text(
          message,
          textAlign: TextAlign.center,
          style: TextStyle(
            color: KarryamColors.indigo.forInk(context),
            height: 1.5,
          ),
        ),
      ],
    ),
  );
}

class _PriceResult extends StatelessWidget {
  const _PriceResult({super.key, required this.data, required this.food});
  final JsonObject data;
  final bool food;
  @override
  Widget build(BuildContext context) {
    final currency = data['currency'],
        amount =
            data[food ? 'effective_unit_price_minor' : 'total_price_minor'];
    final valid = food || data['order_rules_satisfied'] == true;
    return SettingsCard(
      color: (valid ? const Color(0xFFEAF8F3) : const Color(0xFFFFF6E8))
          .forSurface(context),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                valid ? Icons.receipt_long_rounded : Icons.info_outline_rounded,
                color: (valid ? KarryamColors.tealInk : const Color(0xFF986013))
                    .forInk(context),
                size: 28,
              ),
              const Spacer(),
              Text(
                food ? 'Per item' : '${data['quantity']} units',
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
            ],
          ),
          const SizedBox(height: 20),
          Text(
            amount == null
                ? 'Price unavailable'
                : dashboardMoney(amount, currency),
            style: const TextStyle(
              fontSize: 30,
              fontWeight: FontWeight.w700,
              letterSpacing: -.8,
            ),
          ),
          const SizedBox(height: 22),
          if (food) ...[
            SettingsFact(
              'Base price',
              dashboardMoney(data['base_price_minor'], currency),
            ),
            SettingsFact('Price source', dashboardLabel(data['base_source'])),
            for (final m in data['modifiers'] as List)
              SettingsFact(
                m['name'].toString(),
                dashboardMoney(m['price_delta_minor'], currency),
              ),
            SettingsFact(
              'Extras total',
              dashboardMoney(data['modifier_total_minor'], currency),
            ),
          ] else ...[
            SettingsFact(
              'Price per unit',
              data['unit_price_minor'] == null
                  ? 'Unavailable'
                  : dashboardMoney(data['unit_price_minor'], currency),
            ),
            SettingsFact('Price source', dashboardLabel(data['price_source'])),
            SettingsFact('Channel', dashboardLabel(data['mode'])),
            SettingsFact('Minimum quantity', data['minimum_order_quantity']),
            SettingsFact('Order multiple', data['order_multiple']),
            if (data['maximum_order_quantity'] != null)
              SettingsFact('Maximum quantity', data['maximum_order_quantity']),
            SettingsFact('Order rules', valid ? 'Satisfied' : 'Not satisfied'),
            SettingsFact(
              'Stock availability',
              data['availability'] == 'UNAVAILABLE'
                  ? 'Not provided'
                  : dashboardLabel(data['availability']),
            ),
            if (data['reason'] != null)
              Padding(
                padding: const EdgeInsets.only(top: 12),
                child: Text(
                  data['reason'].toString(),
                  style: TextStyle(
                    color: Color(0xFF986013).forInk(context),
                    height: 1.5,
                  ),
                ),
              ),
          ],
        ],
      ),
    );
  }
}
