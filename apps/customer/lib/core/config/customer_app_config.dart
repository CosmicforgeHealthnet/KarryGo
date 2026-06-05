class CustomerAppConfig {
  const CustomerAppConfig({required this.customerApiBaseUrl});

  static const defaultCustomerApiBaseUrl =
      'http://localhost:8101/api/v1/customer';

  factory CustomerAppConfig.fromEnvironment() {
    return const CustomerAppConfig(
      customerApiBaseUrl: String.fromEnvironment(
        'CUSTOMER_API_BASE_URL',
        defaultValue: defaultCustomerApiBaseUrl,
      ),
    );
  }

  final String customerApiBaseUrl;
}
