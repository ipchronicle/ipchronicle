export const en = {
  translation: {
    appName: "IPChronicle",
    common: {
      cancel: "Cancel",
    },
    navigation: {
      menu: "Primary navigation",
      mobileDescription: "Browse IPChronicle sections.",
      toggleSidebar: "Toggle sidebar",
      systemStatus: "System status",
      nodes: "Nodes",
      history: "History",
      account: "Account",
      settings: "Settings",
      networkSettings: "Network probes",
      historySettings: "History and storage",
    },
    language: {
      current: "EN",
      switch: "Switch to Simplified Chinese",
    },
    theme: {
      dark: "Use dark theme",
      light: "Use light theme",
    },
    authentication: {
      section: "Administrator",
      loginTitle: "Sign in",
      login: "Sign in",
      logout: "Sign out",
    },
    settings: {
      section: "Settings",
    },
    account: {
      title: "Account and security",
      profile: "Administrator account",
      profileDetail: "Update the local administrator identity.",
      username: "Username",
      password: "Password",
      currentPassword: "Current password",
      newPassword: "New password",
      save: "Save changes",
      saved: "Account changes saved.",
      language: "Language",
      languageDetail: "Saved for this administrator.",
    },
    totp: {
      title: "Two-factor authentication",
      code: "Six-digit code",
      active: "TOTP is enabled.",
      inactive: "TOTP is disabled.",
      enable: "Enable TOTP",
      confirm: "Confirm TOTP",
      enabled: "TOTP enabled.",
      disable: "Disable TOTP",
      qrCode: "TOTP enrollment QR code",
      secret: "Setup key",
      copySecret: "Copy setup key",
      copied: "Setup key copied.",
    },
    sessions: {
      title: "Administrator sessions",
      detail: "The current session expires after 30 days.",
      revokeAll: "Sign out all sessions",
      confirmTitle: "Sign out every administrator session?",
      confirm: "Sign out all",
    },
    status: {
      section: "Center",
      title: "System status",
      checking: "Checking center status",
      operational: "Operational",
      unavailable: "Unavailable",
      healthyDetail: "The center API and both databases are ready.",
      errorDetail: "The center status could not be loaded.",
      service: "Service",
      version: "Version",
      configSchema: "Configuration schema",
      historySchema: "History schema",
      transport: "Browser transport",
      externalOrigin: "External origin",
      trustedProxy: "Trusted proxy",
      configured: "Configured",
      notConfigured: "Not configured",
      checkedAt: "Checked",
      notAvailable: "Not available",
      retry: "Retry",
      defaultCredentialsTitle: "Default credentials are still active",
      defaultCredentialsDetail:
        "The administrator username and password are both admin.",
      openAccount: "Open account settings",
      httpWarningTitle: "Browser connection uses HTTP",
      httpWarningDetail:
        "Credentials, TOTP codes, and sessions are not protected from network interception.",
    },
    nodes: {
      section: "Managed nodes",
      title: "Nodes",
      detail:
        "Enroll Linux Agents and review their current control-plane state.",
      refresh: "Refresh",
      retry: "Retry",
      loadFailed: "Nodes could not be loaded",
      notAvailable: "Not yet reported",
      enrollment: {
        title: "Agent enrollment",
        detail:
          "Control automatic registration and install an Agent with one root command.",
        enabled: "Enrollment enabled",
        disabled: "Enrollment disabled",
        allowRegistration: "Allow automatic registration",
        allowRegistrationDetail:
          "Disabling enrollment does not disconnect Agents that are already registered.",
        command: "Installation command",
        commandDetail:
          "Run as root on a supported Linux node. The command contains the reusable registration key.",
        copy: "Copy command",
        copied: "Installation command copied.",
        rotatedAt: "Key rotated {{value}}",
        rotate: "Rotate key",
        rotateTitle: "Rotate the registration key?",
        rotateDetail:
          "Previously copied installation commands will stop working. Existing Agents are unaffected.",
        rotateConfirm: "Rotate key",
        rotated: "Registration key rotated.",
        generated: "Registration key generated and enrollment enabled.",
        empty: "No registration key exists",
        emptyDetail:
          "Generate one to enable automatic node creation and receive the installation command.",
        generate: "Generate key",
      },
      inventory: {
        title: "Node inventory",
        detail:
          "A node appears after its Agent completes automatic registration.",
        count_one: "{{count}} node",
        count_other: "{{count}} nodes",
        empty: "No nodes are registered",
        emptyDetail:
          "Generate an enrollment key, then run the installation command on a Linux node you manage.",
        node: "Node",
        status: "Status",
        agent: "Agent",
        configuration: "Configuration",
        lastSeen: "Last seen",
      },
      status: {
        online: "Online",
        offline: "Offline",
        disabled: "Disabled",
        revoked: "Revoked",
      },
      configuration: {
        current: "Current",
        pending: "Pending",
        failed: "Failed",
      },
      sync: {
        start: "Start temporary sync",
        stop: "Stop temporary sync",
        unsupported:
          "The current Agent version does not support temporary sync",
        pending: "Waiting for Agent",
        connected: "Live sync active",
        degraded: "Using normal polling",
        until: "Ends",
      },
      actions: {
        title: "Node actions",
        network: "Network egresses",
        probe: "Complete probes",
        enable: "Enable node",
        disable: "Pause node",
        revoke: "Revoke Agent credential",
        delete: "Permanently delete node",
        retryDeletion: "Retry permanent deletion",
      },
      revoke: {
        title: "Revoke the Agent credential for {{name}}?",
        detail:
          "This cannot be undone. The Agent stops control polling and local schedules after its next connection; node configuration and history remain available.",
        confirm: "Revoke credential",
      },
      deletion: {
        title: "Permanently delete {{name}}?",
        detail:
          "The node, egresses, current state, history, starred snapshots, and related notification records are irreversibly removed. The Center does not uninstall the Agent service from the host.",
        confirm: "Permanently delete",
        pending: "Deleting",
        failed: "Deletion failed",
      },
    },
    proxySettings: {
      section: "Settings",
      title: "Network probes",
      detail:
        "Manage reusable HTTP, HTTPS, and SOCKS5 proxies. Passwords can be replaced or cleared but are never shown again.",
      refresh: "Refresh",
      retry: "Retry",
      loadFailed: "Network proxy settings could not be loaded",
      empty: "No centrally managed proxies are configured.",
      save: "Save settings",
      discovery: {
        title: "Public address discovery",
        detail:
          "The Agent tries these independent services in order. First observations and suspected changes require agreement from two services.",
        ipv4: "IPv4 services",
        ipv6: "IPv6 services",
        format:
          "Enter one HTTP or HTTPS URL per line. Each list requires 2 to 8 distinct hosts.",
        save: "Save discovery services",
        updated: "Updated {{value}}",
        httpTitle: "A discovery service uses HTTP",
        httpDetail:
          "An unencrypted response can be changed in transit and may create a false address event. Saving remains allowed.",
      },
      create: {
        title: "Add proxy",
        detail:
          "The Center sends credentials only to Agents with an egress that references this proxy.",
        submit: "Add proxy",
      },
      fields: {
        name: "Name",
        scheme: "Protocol",
        host: "Host or IP address",
        port: "Port",
        username: "Username",
        password: "Password",
        passwordPlaceholder: "Leave empty to keep the current password",
      },
      password: {
        configured: "Password configured",
        empty: "No password",
        replace: "Replace password",
        clear: "Clear password",
        clearTitle: "Clear the stored proxy password?",
        clearDetail:
          "Agents referencing this proxy will receive a new configuration without the password.",
      },
      delete: {
        action: "Delete proxy",
        title: "Delete this network proxy?",
        detail:
          "A proxy referenced by any network egress cannot be deleted. Remove those egresses first.",
        confirm: "Delete proxy",
      },
    },
    network: {
      section: "Node network",
      title: "Network egresses",
      detail:
        "Review Agent-reported interfaces, addresses, and routes, then manage the network paths this node can probe.",
      back: "Back to nodes",
      refresh: "Refresh",
      retry: "Retry",
      loadFailed: "Node network state could not be loaded",
      nodeNotFound: "The node does not exist or was deleted",
      family: { ipv4: "IPv4", ipv6: "IPv6" },
      egresses: {
        title: "Configured egresses",
        detail:
          "Egresses are durable probe paths. A missing selector does not delete its configuration.",
        empty:
          "Default egresses appear after the Agent reports usable default routes.",
        available: "Available",
        unavailable: "Unavailable",
        automatic: "Auto-discovered",
        default: { ipv4: "Default IPv4", ipv6: "Default IPv6" },
        interface: "Interface {{name}}",
        source: "{{name}} · {{address}}",
        proxy: "Proxy · {{name}}",
        missingProxy: "Missing proxy",
        enabledLabel: "Enable or disable {{name}}",
        interval: "Address check interval (seconds)",
        saveInterval: "Save address check interval",
        probeOnChange: "Probe after a confirmed address change",
        probeOnChangeDetail:
          "The first confirmed address does not start a complete probe.",
        delete: "Permanently delete egress",
        deleteTitle: "Permanently delete this network egress?",
        deleteDetail:
          "The egress configuration and its owned data will be irreversibly deleted. An automatic egress may be created with a new identity while its default route remains usable; use the switch to pause it instead.",
        deleteConfirm: "Permanently delete",
        deletionPending: "Deletion pending",
        deletionFailed: "Deletion failed",
        retryDeletion: "Retry permanent deletion",
      },
      observation: {
        waiting: "Waiting for the first lightweight address observation.",
        unknown: "No confirmed public address",
        proxy: "Proxy path",
        nat: "Likely NAT",
        temporary: "Temporary IPv6 source",
        natDetail:
          "The local source differs from the observed public address. Some upstream DNS or raw mail checks may use the default route or fail to bind.",
        status: { confirmed: "Confirmed", failed: "Check failed" },
        failure: {
          "selector-unavailable":
            "The configured local selector is unavailable.",
          "no-valid-response": "No discovery service returned a valid address.",
          "confirmation-unavailable":
            "A second independent service could not confirm the address.",
          "conflicting-responses":
            "Independent discovery services returned different addresses.",
        },
      },
      addressHistory: {
        title: "Address transitions",
        detail:
          "Only first observations, confirmed changes, failure boundaries, recoveries, and reported gaps are retained.",
        empty: "No address transition has been reported.",
        kind: {
          "first-observation": "First observation",
          "address-change": "Address changed",
          "check-failure": "Check failed",
          recovery: "Recovered",
        },
        gap: "History gap",
        gapDetail:
          "{{count}} offline events were discarded (sequence {{first}} to {{last}}).",
      },
      proxyEgress: {
        title: "Add proxy egress",
        detail:
          "Create a durable IPv4 or IPv6 path through a centrally managed proxy.",
        empty: "Configure a reusable proxy before adding a proxy egress.",
        openSettings: "Open network settings",
        proxy: "Proxy",
        family: "Address family",
        add: "Add egress",
        configured: "Already configured",
      },
      candidates: {
        title: "Discovered candidate paths",
        detail:
          "Only stable interfaces and source addresses in the current Agent inventory can be enabled.",
        empty: "No unconfigured candidate paths are available.",
        interface: "Interface {{name}}",
        source: "{{name}} · {{address}}",
        temporary: "Temporary IPv6",
        add: "Enable path",
      },
      inventory: {
        title: "Interface inventory",
        detail:
          "Shows the last valid Agent capture. A collection failure does not overwrite it.",
        empty: "The Agent has not reported a valid network inventory.",
        failed: "The latest network inventory collection failed",
        interface: "Interface",
        index: "Index",
        state: "State",
        up: "Up",
        down: "Down",
        loopback: "Loopback",
      },
      addresses: {
        title: "Addresses",
        detail:
          "Addresses stay associated with their interface and kernel lifecycle flags.",
        address: "Address",
        scope: "Scope",
        lifecycle: "Lifecycle",
        stable: "Stable",
        temporary: "Temporary",
        tentative: "Tentative",
        deprecated: "Deprecated",
        duplicate: "Duplicate",
      },
      routes: {
        title: "Routes",
        detail:
          "The Agent reports IPv4 and IPv6 routes currently marked up by the kernel.",
        destination: "Destination",
        gateway: "Gateway",
        metric: "Metric",
        default: { ipv4: "Default IPv4", ipv6: "Default IPv6" },
      },
      scope: {
        global: "Global",
        private: "Private",
        shared: "Shared space",
        "unique-local": "IPv6 unique-local",
        "link-local": "Link-local",
        loopback: "Loopback",
        multicast: "Multicast",
        unspecified: "Unspecified",
        other: "Other",
      },
      reason: {
        "interface-down": "The interface is down",
        "no-usable-route": "No usable route or address is available",
        "temporary-address":
          "A temporary IPv6 address cannot become its own durable egress",
        "unusable-address": "The address is not currently usable",
      },
    },
    probe: {
      section: "Node probes",
      title: "Complete probes",
      detail: "Schedule and inspect full IPQuality runs for this node.",
      back: "Back to nodes",
      refresh: "Refresh",
      retry: "Retry",
      loadFailed: "Complete-probe state could not be loaded",
      nodeNotFound: "The node does not exist or was deleted",
      notAvailable: "Not available",
      runNow: "Run complete probe",
      lowMemory: {
        title: "Complete probes are paused for low memory",
        detail:
          "The Agent reported less than 256 MiB of physical memory. Address checks continue; an administrator can accept the risk in probe settings.",
      },
      unavailable: {
        offline:
          "The node is offline, so an immediate task cannot be delivered.",
        disabled:
          "The node is disabled. Enable it before starting a complete probe.",
        lowMemory:
          "Complete probes are paused until the low-memory override is enabled.",
        running: "A complete-probe run is already active on this node.",
        task: "The node already has an immediate task waiting or running.",
      },
      status: {
        title: "Agent probe state",
        detail: "Latest schedule and execution state reported by the Agent.",
        running: "Running",
        idle: "Idle",
        next: "Next scheduled run",
        last: "Last occurrence",
        trigger: "Trigger",
        memory: "Physical memory",
        lastSkipped: "The last occurrence was skipped: {{reason}}",
        resetApplied:
          "History reset applied {{value}}; {{count}} queued item(s) were discarded.",
      },
      task: {
        title: "Immediate task",
        detail:
          "Delivery and execution state for the latest administrator command.",
        empty: "No immediate complete-probe task has been created.",
        created: "The task is waiting for the Agent.",
        offline: "The task is still active, but the node is currently offline.",
        waiting: "Waiting for Agent",
        createdAt: "Created",
        receivedAt: "Agent received",
        startedAt: "Started",
        completedAt: "Completed",
        openRun: "Open run",
      },
      runs: {
        title: "Recent runs",
        detail:
          "Node-level runs retain their frozen egress order and individual outcomes.",
        empty: "No complete probe has been run yet.",
        progress: "{{completed}} of {{total}} complete",
      },
      settings: {
        title: "Probe settings",
        detail:
          "The schedule runs locally on the Agent using a six-field Cron expression.",
        scheduleEnabled: "Enable recurring probes",
        scheduleEnabledDetail:
          "Missed occurrences are skipped and are not run after restart.",
        cron: "Cron expression",
        timezone: "Time zone",
        memoryOverride: "Allow probes below 256 MiB",
        memoryOverrideDetail:
          "Enabling this accepts possible probe failure or node resource exhaustion.",
        save: "Save probe settings",
      },
      trigger: {
        manual: "Manual",
        schedule: "Schedule",
        "address-change": "Address change",
      },
      state: {
        pending: "Waiting",
        acknowledged: "Received",
        running: "Running",
        succeeded: "Succeeded",
        partial: "Partial success",
        failed: "Failed",
        rejected: "Rejected",
        expired: "Expired",
        interrupted: "Interrupted",
        skipped: "Skipped",
      },
      skip: {
        busy: "another probe was active",
        disabled: "probing was disabled",
        "low-memory": "the node was below the memory baseline",
        "no-egress": "no enabled egress was available",
        missed: "the occurrence was missed",
      },
      failure: {
        download: "Script download",
        selector: "Egress selector",
        adapter: "Proxy adapter",
        process: "Probe process",
        timeout: "Timeout",
        output: "JSON output",
        restart: "Agent restart",
      },
    },
    probeRun: {
      section: "Probe run",
      title: "Complete-probe run",
      detail: "Run details",
      back: "Back to node probes",
      refresh: "Refresh",
      retry: "Retry",
      notFound: "The probe run does not exist or its history was removed",
      loadFailed: "The probe run could not be loaded",
      partial: {
        title: "This run completed with partial success",
        detail:
          "Successful egress snapshots remain available alongside failed or skipped sibling executions.",
      },
      summary: {
        title: "Run summary",
        trigger: "Trigger",
        startedAt: "Started",
        completedAt: "Completed",
        progress: "Progress",
      },
      executions: {
        title: "Egress executions",
        detail: "Each frozen egress is attempted at most once in this run.",
        sequence: "Egress sequence {{value}}",
        startedAt: "Started",
        completedAt: "Completed",
        stage: "Failure stage",
        openSnapshot: "Open report snapshot",
        snapshotPending: "The successful snapshot is still arriving.",
      },
    },
    history: {
      section: "Cross-node index",
      title: "History",
      detail:
        "Browse complete-probe reports and address transitions by node, network egress, and time.",
      retry: "Retry",
      loadFailed: "History could not be loaded",
      tabs: {
        label: "History type",
        reports: "Probe reports",
        addresses: "Address changes",
      },
      filters: {
        title: "Filters",
        detail: "Active filters are retained in the current address.",
        clear: "Clear filters",
        node: "Node",
        allNodes: "All nodes",
        egress: "Network egress",
        allEgresses: "All egresses",
        from: "From",
        to: "To",
        runStatus: "Run result",
        allResults: "All results",
        trigger: "Trigger",
        allTriggers: "All triggers",
        changes: "Field changes",
        allChanges: "All",
        changed: "Changed",
        unchanged: "Unchanged",
        format: "Upstream format",
        allFormats: "All formats",
        eventKind: "Event type",
        allEvents: "All events",
        family: "Address family",
        allFamilies: "All families",
      },
      format: { compatible: "Compatible", mismatch: "Format mismatch" },
      reports: {
        title: "Complete-probe reports",
        count: "{{count}} retained snapshots",
        open: "Open report",
        comparePrevious: "Compare with previous",
        baseline: "First baseline",
        changeCount: "{{count}} changes",
        noChanges: "No field changes",
        formatIssues: "{{count}} format issues",
        current: "Current snapshot",
      },
      addresses: {
        title: "Address changes",
        count: "{{count}} events",
        previous: "Previous: {{value}}",
      },
      columns: {
        owner: "Node and egress",
        result: "Run result",
        interpretation: "Interpretation",
        time: "Observed",
        actions: "Actions",
        event: "Event",
        path: "Local to public",
      },
      starred: "Starred",
      compareSelection: {
        title: "First snapshot selected",
        detail: "The list is now limited to the same network egress.",
        clear: "Clear selection",
        selected: "Selected",
        compare: "Compare",
        differentEgress: "Different egress",
        select: "Select to compare",
      },
      gaps: {
        probeTitle: "Probe history gaps",
        addressTitle: "Address history gaps",
        detail:
          "Data dropped from a bounded Agent queue is never presented as continuous history.",
        probeItem: "{{count}} results missing, sequence {{first}} to {{last}}",
        addressItem: "{{count}} events missing, sequence {{first}} to {{last}}",
      },
      formatEvents: {
        title: "Upstream format events",
        detail:
          "Format mismatches, changes, and recoveries found by the fixed field catalog.",
        issueCount: "{{count}} issues",
        kind: {
          mismatch: "Format mismatch detected",
          changed: "Mismatch changed",
          recovered: "Format recovered",
        },
      },
      pagination: {
        previous: "Previous",
        next: "Next",
        page: "Page {{current}} of {{total}}",
      },
      empty: { filtered: "No matching records", none: "No history yet" },
    },
    comparison: {
      back: "Back to history",
      section: "Probe reports",
      title: "Snapshot comparison",
      detail: "Compare two retained snapshots from the same network egress.",
      invalid: "Snapshots to compare were not provided",
      notFound: "A snapshot does not exist or was removed",
      egressMismatch: "The snapshots belong to different network egresses",
      loadFailed: "The comparison could not be loaded",
      retry: "Retry",
      before: "Before",
      after: "After",
      summary: {
        title: "Comparison scope",
        egress: "Network egress {{value}}",
      },
      changed: {
        title: "Changed fields",
        count: "{{count}} semantic changes",
        empty: "No compatible field changed",
        badge: "Changed",
      },
      unchanged: {
        title: "Unchanged fields",
        count: "{{count}} fields",
        show: "Show unchanged fields",
      },
    },
    snapshot: {
      section: "Probe report",
      title: "Report snapshot",
      back: "Back",
      retry: "Retry",
      notFound: "The snapshot does not exist or was removed",
      loadFailed: "The snapshot could not be loaded",
      star: "Star snapshot",
      unstar: "Unstar snapshot",
      comparePrevious: "Compare with previous",
      unavailable: "Unavailable",
      actualType: "Actual type {{actual}}; expected {{expected}}",
      summary: {
        title: "Interpretation summary",
        baseline: "First baseline",
        changes: "{{count}} changes",
        noChanges: "No field changes",
        formatIssues: "{{count}} format issues",
        compatible: "Format compatible",
      },
      format: {
        title: "Upstream output format mismatch",
        detail:
          "Incompatible fields are not coerced. Unknown fields remain available in the raw JSON.",
      },
      views: {
        label: "Report view",
        structured: "Structured fields",
        raw: "Raw JSON",
      },
      structured: { fieldCount: "{{count}} known fields" },
      fieldStatus: {
        available: "Available",
        missing: "Missing",
        incompatible: "Incompatible type",
      },
      issueKind: {
        missing: "Missing field",
        incompatible: "Incompatible type",
        unknown: "Unknown field",
      },
      changes: {
        title: "Changes in this snapshot",
        detail:
          "Only semantic changes with compatible types on both sides are shown.",
      },
      fieldCatalog: {
        unmappedDescription:
          "Known upstream field without a localized description.",
        groups: {
          head: {
            name: "Report metadata",
            description:
              "Identity and generation details from the upstream report.",
          },
          info: {
            name: "IP information",
            description:
              "Network ownership and geographic information for the address.",
          },
          type: {
            name: "Network classification",
            description:
              "Usage and organization classifications from IP databases.",
          },
          score: {
            name: "Risk scores",
            description: "Risk scores reported by upstream data providers.",
          },
          factor: {
            name: "Risk indicators",
            description:
              "Country and risk signals reported by upstream data providers.",
          },
          media: {
            name: "Media services",
            description:
              "Service availability, region, and result classifications.",
          },
          mail: {
            name: "Mail connectivity",
            description: "Outbound mail reachability and DNS blocklist checks.",
          },
        },
        fields: {
          head: {
            ip: {
              name: "IP address",
              description: "Public IP address reported by the upstream probe.",
            },
            command: {
              name: "Probe command",
              description: "Command recorded by the upstream probe.",
            },
            github: {
              name: "Upstream source",
              description: "Source reference recorded by the upstream probe.",
            },
            time: {
              name: "Report time",
              description: "Generation time recorded in the upstream report.",
            },
            version: {
              name: "Probe version",
              description: "Version reported by the upstream probe.",
            },
          },
          info: {
            asn: {
              name: "Autonomous system number",
              description:
                "Autonomous system number associated with the address.",
            },
            organization: {
              name: "Organization",
              description: "Organization associated with the address.",
            },
            latitude: {
              name: "Latitude",
              description: "Reported geographic latitude.",
            },
            longitude: {
              name: "Longitude",
              description: "Reported geographic longitude.",
            },
            dms: {
              name: "DMS coordinates",
              description: "Location in degrees, minutes, and seconds.",
            },
            map: {
              name: "Map reference",
              description: "Upstream map reference for the reported location.",
            },
            timeZone: {
              name: "Time zone",
              description: "Time zone associated with the address.",
            },
            cityName: {
              name: "City",
              description: "Reported city or locality name.",
            },
            cityPostalCode: {
              name: "Postal code",
              description: "Reported postal code for the city or locality.",
            },
            citySubCode: {
              name: "City subdivision code",
              description:
                "Reported subdivision code within the city or locality.",
            },
            citySubdivisions: {
              name: "City subdivisions",
              description: "Reported subdivisions within the city or locality.",
            },
            regionCode: {
              name: "Region code",
              description: "Code of the reported geographic region.",
            },
            regionName: {
              name: "Region",
              description: "Name of the reported geographic region.",
            },
            continentCode: {
              name: "Continent code",
              description: "Code of the reported continent.",
            },
            continentName: {
              name: "Continent",
              description: "Name of the reported continent.",
            },
            registeredRegionCode: {
              name: "Registered-region code",
              description:
                "Code of the region where the address is registered.",
            },
            registeredRegionName: {
              name: "Registered region",
              description:
                "Name of the region where the address is registered.",
            },
            type: {
              name: "Address type",
              description: "Network or address type reported upstream.",
            },
          },
          classification: {
            usage: {
              name: "Usage classification ({{provider}})",
              description:
                "Address usage classification reported by {{provider}}.",
            },
            company: {
              name: "Company classification ({{provider}})",
              description:
                "Organization classification reported by {{provider}}.",
            },
          },
          score: {
            risk: {
              name: "Risk score ({{provider}})",
              description: "Address risk score reported by {{provider}}.",
            },
          },
          factor: {
            countryCode: {
              name: "Country code ({{provider}})",
              description: "Address country code reported by {{provider}}.",
            },
            proxy: {
              name: "Proxy indicator ({{provider}})",
              description:
                "Whether {{provider}} identifies the address as a proxy.",
            },
            tor: {
              name: "Tor indicator ({{provider}})",
              description:
                "Whether {{provider}} identifies the address as a Tor exit.",
            },
            vpn: {
              name: "VPN indicator ({{provider}})",
              description:
                "Whether {{provider}} identifies the address as a VPN.",
            },
            server: {
              name: "Server indicator ({{provider}})",
              description:
                "Whether {{provider}} identifies the address as a server.",
            },
            abuser: {
              name: "Abuse indicator ({{provider}})",
              description:
                "Whether {{provider}} associates the address with abuse.",
            },
            robot: {
              name: "Automation indicator ({{provider}})",
              description:
                "Whether {{provider}} associates the address with automation.",
            },
          },
          media: {
            status: {
              name: "{{service}} availability",
              description: "Availability result reported for {{service}}.",
            },
            region: {
              name: "{{service}} region",
              description: "Service region reported for {{service}}.",
            },
            type: {
              name: "{{service}} result type",
              description: "Result classification reported for {{service}}.",
            },
          },
          mail: {
            connectivity: {
              name: "{{service}} connectivity",
              description: "Outbound mail connectivity result for {{service}}.",
            },
            dnsTotal: {
              name: "DNS blocklists checked",
              description: "Total number of DNS blocklists checked.",
            },
            dnsClean: {
              name: "Clean DNS blocklists",
              description:
                "Number of DNS blocklists that did not list the address.",
            },
            dnsMarked: {
              name: "Marked DNS blocklists",
              description: "Number of DNS blocklists that marked the address.",
            },
            dnsBlacklisted: {
              name: "Blacklisted DNS blocklists",
              description:
                "Number of DNS blocklists that blacklisted the address.",
            },
          },
        },
      },
      raw: {
        title: "IPQuality JSON",
        detail: "Egress sequence {{sequence}}, observed {{value}}",
        wrap: "Toggle line wrapping",
        copy: "Copy JSON",
        copied: "Copied",
        download: "Download JSON",
      },
    },
    historySettings: {
      title: "History and storage",
      detail:
        "Manage retention, inspect logical and physical usage, and clean observed data.",
      retry: "Retry",
      loadFailed: "History state could not be loaded",
      usage: {
        title: "Storage usage",
        detail:
          "Logical usage drives retention. Physical usage includes the SQLite database, WAL, and shared-memory files.",
        logical: "Logical usage",
        protected: "Protected usage",
        records: "History records",
        physical: "Physical usage",
        overBudget: "History usage exceeds the configured limit",
        overage:
          "Usage is {{value}} over budget. Stars, current snapshots, or active runs may prevent further cleanup.",
      },
      retention: {
        title: "Retention policy",
        detail:
          "Starred snapshots, current snapshots, current format state, and active runs are never removed automatically.",
        mode: "Policy",
        indefinite: "Keep indefinitely",
        age: "Keep by age",
        size: "Keep by logical usage",
        days: "Retention days",
        mib: "Logical limit (MiB)",
        updated: "Last updated: {{value}}",
        save: "Save and apply",
        invalid: "The retention policy values are invalid.",
      },
      cleanup: {
        action: "Clean now",
        lastRun: "Last cleanup",
        never: "Not yet run",
        lastDeleted: "Last deleted items",
        failed: "The last cleanup failed",
      },
      state: {
        title: "History state",
        detail:
          "The generation prevents offline Agent queues from restoring intentionally cleared history.",
        generation: "Current generation",
        resetAt: "Last cleared",
        never: "Never cleared",
      },
      feedback: {
        saved: "The retention policy was saved and applied.",
        cleaned: "Cleanup completed and removed {{count}} items.",
        cleared: "Observed history was cleared.",
      },
      danger: {
        title: "Clear observed history",
        detail:
          "This removes address events, probe runs, executions, snapshots, and history gaps. Node, egress, proxy, account, and task configuration remains.",
        action: "Clear history",
        confirmTitle: "Clear all observed history?",
        confirmDetail:
          "This action cannot be undone. Agents will discard queued data from the old generation; no complete probe starts automatically.",
        confirm: "Clear all history",
      },
    },
    errors: {
      actionFailed: "The action could not be completed.",
      invalid_request: "The submitted values are invalid.",
      invalid_credentials: "The username, password, or code is incorrect.",
      totp_required: "Enter the six-digit authentication code.",
      rate_limited:
        "Too many attempts. Try again in {{retryAfterSeconds}} seconds.",
      unauthenticated: "The administrator session has expired.",
      csrf_failed:
        "The security token is invalid. Reload the page and try again.",
      origin_not_allowed: "The request origin is not allowed.",
      current_password_invalid: "The current password is incorrect.",
      invalid_totp: "The authentication code is invalid or was already used.",
      totp_already_enabled: "TOTP is already enabled.",
      totp_not_enabled: "TOTP is not enabled.",
      totp_enrollment_not_started:
        "Start TOTP enrollment before confirming it.",
      no_account_change: "Change the username or enter a new password.",
      registration_key_not_initialized:
        "Generate a registration key before enabling enrollment.",
      registration_key_invalid: "The registration key is invalid.",
      registration_disabled: "Automatic Agent enrollment is disabled.",
      agent_unauthenticated: "The Agent credential is invalid.",
      agent_revoked: "The Agent credential has been revoked.",
      node_not_found: "The node does not exist or has already been deleted.",
      node_revoked: "A node with a revoked Agent credential cannot be enabled.",
      node_deletion_pending:
        "The node is being permanently deleted and cannot accept other changes.",
      node_sync_unsupported:
        "The current Agent version does not support temporary sync.",
      sync_session_unavailable:
        "The temporary sync session has ended or is unavailable.",
      network_inventory_unavailable:
        "The node has not reported a valid network inventory.",
      invalid_egress_candidate:
        "This interface or source address cannot currently become a network egress.",
      egress_already_exists: "This network egress already exists.",
      egress_limit_reached:
        "This node has reached the maximum of 64 configured network egresses.",
      egress_not_found: "The network egress does not exist or was deleted.",
      egress_deletion_pending:
        "The network egress is being permanently deleted and cannot accept other changes.",
      invalid_network_proxy: "The network proxy settings are invalid.",
      network_proxy_not_found:
        "The network proxy does not exist or was deleted.",
      network_proxy_already_exists:
        "A network proxy with this name already exists.",
      network_proxy_limit_reached:
        "The installation has reached the maximum of 64 network proxies.",
      network_proxy_in_use:
        "This proxy is still referenced by a network egress and cannot be deleted.",
      invalid_observation_settings:
        "Use 2 to 8 valid HTTP or HTTPS URLs with distinct hosts for each address family.",
      invalid_probe_settings: "The Cron expression or time zone is invalid.",
      node_offline: "The node is offline and cannot receive an immediate task.",
      probe_task_slot_occupied:
        "An immediate task is already waiting or running for this node.",
      probe_already_running:
        "A complete-probe run is already active on this node.",
      probe_paused_low_memory:
        "Complete probes are paused because this node reported less than 256 MiB of memory.",
      no_enabled_egress: "This node has no enabled network egress.",
      probe_run_not_found:
        "The complete-probe run does not exist or was removed.",
      probe_snapshot_not_found:
        "The report snapshot does not exist or was removed.",
      snapshot_egress_mismatch:
        "The snapshots belong to different network egresses.",
      internal_error: "The center could not complete the request.",
    },
  },
} as const;
