import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import 'order_form_sheet.dart';
import 'order_design.dart';

class OrderSubstitutionScreen extends StatefulWidget {
  const OrderSubstitutionScreen({
    super.key,
    required this.runtime,
    required this.branchId,
  });
  final VendorRuntime runtime;
  final String branchId;
  @override
  State<OrderSubstitutionScreen> createState() =>
      _OrderSubstitutionScreenState();
}

class _OrderSubstitutionScreenState extends State<OrderSubstitutionScreen> {
  late Future<List<JsonObject>> _future = _load();
  Future<List<JsonObject>> _load() async {
    final api = widget.runtime.backend.shopCatalogueApi;
    final response = await api.getShopBranchesBranchIdListings(
      branchId: widget.branchId,
      query: {'status': 'ACTIVE', 'limit': '50', 'offset': '0'},
    );
    final items = response.object['items'];
    if (items is! List) {
      throw const FormatException('Incomplete listing response');
    }
    return Future.wait(
      items.map((raw) async {
        final listing = jsonObject(raw);
        final product = await api.getShopProductsProductId(
          productId: listing['product_ref'].toString(),
        );
        final variant = await api.getShopVariantsVariantId(
          variantId: listing['variant_ref'].toString(),
        );
        return {
          'listing': listing,
          'product': product.object,
          'variant': variant.object,
        };
      }),
    );
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    backgroundColor: Color(0xFFF5F7FA).forSurface(context),
    appBar: AppBar(
      title: const Text('Choose a replacement'),
      backgroundColor: Color(0xFFF5F7FA).forSurface(context),
      surfaceTintColor: Colors.transparent.forSurface(context),
    ),
    body: SafeArea(
      top: false,
      child: FutureBuilder<List<JsonObject>>(
        future: _future,
        builder: (context, snapshot) {
          if (snapshot.hasError) {
            return SingleChildScrollView(
              padding: const EdgeInsets.all(24),
              child: DashboardEmptyState(
                story:
                    snapshot.error is ApiException &&
                        (snapshot.error as ApiException).statusCode == 403
                    ? EmptyStory.restricted
                    : EmptyStory.unavailable,
                title: 'Replacements unavailable',
                message: 'Available branch listings could not be loaded.',
                action: 'Try again',
                onAction: () => setState(() => _future = _load()),
              ),
            );
          }
          if (!snapshot.hasData) {
            return const PremiumSkeleton(detail: true, rows: 2);
          }
          final rows = snapshot.data!
              .where(
                (row) =>
                    row['product']['status'] == 'ACTIVE' &&
                    row['variant']['status'] == 'ACTIVE',
              )
              .toList();
          return ListView(
            padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
            children: [
              const Text(
                'Suggest a replacement',
                style: TextStyle(fontSize: 25, fontWeight: FontWeight.w700),
              ),
              const SizedBox(height: 10),
              const Text(
                'Choose an active product listing in this branch. The replacement will be checked against the customer\'s policy before it can proceed.',
              ),
              const SizedBox(height: 18),
              if (rows.isEmpty)
                const DashboardEmptyState(
                  story: EmptyStory.products,
                  title: 'No replacements available',
                  message:
                      'Active product listings for this branch will appear here.',
                ),
              for (final row in rows)
                Card(
                  color: Colors.white.forSurface(context),
                  elevation: 0,
                  child: ListTile(
                    leading: OrderIllustration(
                      shop: true,
                      size: 46,
                      color: Color(0xFF285EB0).forInk(context),
                    ),
                    title: Text(row['product']['name']?.toString() ?? ''),
                    subtitle: Text(row['variant']['name']?.toString() ?? ''),
                    trailing: const Icon(Icons.chevron_right_rounded),
                    onTap: () async {
                      final body = await showModalBottomSheet<JsonObject>(
                        context: context,
                        isScrollControlled: true,
                        useSafeArea: true,
                        showDragHandle: true,
                        builder: (_) => const OrderFormSheet(
                          title: 'Send suggestion',
                          message:
                              'The customer may need to approve this replacement. It will not be treated as approved when you send it.',
                          fields: [
                            OrderInput('reason', 'Reason', required: true),
                          ],
                        ),
                      );
                      if (body == null || !context.mounted) return;
                      body['proposed_catalogue_ref'] = row['listing']['id'];
                      Navigator.pop(context, body);
                    },
                  ),
                ),
              const SizedBox(height: 12),
              const Text(
                'Showing up to 50 active listings.',
                style: TextStyle(fontSize: 11),
              ),
            ],
          );
        },
      ),
    ),
  );
}
