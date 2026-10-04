import PartnerCard from "@/components/PartnerCard";

export default function Fleet() {
  return (
    <section
      id="businesses"
      className="relative scroll-mt-20 bg-white py-10 lg:py-14"
    >
      <div className="mx-auto max-w-[1440px] px-6 lg:px-12 grid gap-6 lg:grid-cols-2">
        <PartnerCard
          id="fleet"
          tone="light"
          eyebrow="For Fleets"
          title={
            <>
              Total control.
              <br />
              Maximum efficiency.
            </>
          }
          text="Manage your fleet, drivers, deliveries and operations from a single, powerful platform."
          points={[
            "Organisation & staff",
            "Managed providers",
            "Wallet & payments",
            "CRM",
            "Private operations",
            "Dispatch & control tower",
          ]}
          cta="Explore Fleet"
          href="/waitlist"
          image="/images/fleet.png"
          alt="Karryam fleet dashboard on a laptop with a delivery truck"
          imageWidth={1200}
          imageHeight={1000}
        />

        <PartnerCard
          tone="dark"
          eyebrow="For API Partners"
          title={
            <>
              Smarter logistics
              <br />
              for your businesses.
            </>
          }
          text="A powerful API platform to integrate and scale your business with ease."
          points={[
            "Delivery creation & tracking",
            "Webhooks & events",
            "API keys & sandbox",
            "Billing & usage",
          ]}
          cta="Request API access"
          href="/waitlist"
          image="/images/api-phone.png"
          alt="Karryam API partner console on a phone"
          imageWidth={1000}
          imageHeight={1200}
        />
      </div>
    </section>
  );
}