package commands_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/inscoder/xero-cli/internal/auth"
	"github.com/inscoder/xero-cli/internal/commands"
	appconfig "github.com/inscoder/xero-cli/internal/config"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/xeroapi"
)

const commandContactID = "220ddca8-3144-4085-9a88-2d72c5133734"

type fakeContactClient struct {
	listRequest    xeroapi.ListContactsRequest
	getRequest     xeroapi.GetContactRequest
	createRequest  xeroapi.CreateContactRequest
	updateRequest  xeroapi.UpdateContactRequest
	listResult     xeroapi.ContactListResult
	contact        xeroapi.Contact
	mutationResult xeroapi.ContactMutationResult
	listCalls      int
	createCalls    int
	updateCalls    int
	err            error
}

func (fake *fakeContactClient) ListContacts(_ context.Context, _ auth.TokenSet, request xeroapi.ListContactsRequest) (xeroapi.ContactListResult, error) {
	fake.listRequest = request
	fake.listCalls++
	return fake.listResult, fake.err
}

func (fake *fakeContactClient) GetContact(_ context.Context, _ auth.TokenSet, request xeroapi.GetContactRequest) (xeroapi.Contact, error) {
	fake.getRequest = request
	return fake.contact, fake.err
}

func (fake *fakeContactClient) CreateContact(_ context.Context, _ auth.TokenSet, request xeroapi.CreateContactRequest) (xeroapi.ContactMutationResult, error) {
	fake.createRequest = request
	fake.createCalls++
	result := fake.mutationResult
	if result.Operation == "" {
		result.Operation = "created"
		result.Resource = "contact"
		result.TenantID = request.TenantID
		result.IdempotencyKey = request.IdempotencyKey
	}
	return result, fake.err
}

func (fake *fakeContactClient) UpdateContact(_ context.Context, _ auth.TokenSet, request xeroapi.UpdateContactRequest) (xeroapi.ContactMutationResult, error) {
	fake.updateRequest = request
	fake.updateCalls++
	result := fake.mutationResult
	if result.Operation == "" {
		result.Operation = "updated"
		result.Resource = "contact"
		result.ContactID = request.ContactID
		result.TenantID = request.TenantID
		result.IdempotencyKey = request.IdempotencyKey
	}
	return result, fake.err
}

func TestContactsListMapsFiltersAndEmitsNormalizedJSON(t *testing.T) {
	tempDir := t.TempDir()
	configPath := tempDir + "/config.json"
	prepareConfig(t, configPath)
	prepareSession(t, tempDir+"/session.json")

	contactClient := &fakeContactClient{listResult: xeroapi.ContactListResult{Contacts: []xeroapi.Contact{{
		ContactID:     commandContactID,
		ContactNumber: "CRM-1",
		AccountNumber: "ACME-1",
		ContactStatus: "ACTIVE",
		Name:          "Acme",
		FirstName:     "Alex",
		LastName:      "Morgan",
		CompanyNumber: "COMP-1",
		EmailAddress:  "alex@example.invalid",
		Phones:        []xeroapi.ContactPhone{{PhoneType: "DEFAULT", PhoneNumber: "111"}},
		IsCustomer:    true,
		UpdatedAt:     "2026-07-11T01:02:03Z",
	}}}}
	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	deps, stdout, stderr := testDependencies(configPath, store, &fakeLister{}, false)
	deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient { return contactClient }

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{
		"--config", configPath,
		"contacts", "list",
		"--search", "  Acme  ",
		"--contact-id", strings.ToUpper(commandContactID) + ",88192A99-CBC5-4A66-BF1A-2F9FEA2D36D0",
		"--page", "2",
		"--page-size", "50",
		"--include-archived",
		"--summary-only",
		"--since", "2026-07-01",
		"--where", `  ContactStatus=="ACTIVE"  `,
		"--order", "  Name desc  ",
		"--json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute contacts list: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	request := contactClient.listRequest
	if request.TenantID != "tenant-1" || request.Search != "Acme" || request.Page != 2 || request.PageSize != 50 || !request.IncludeArchived || !request.SummaryOnly || request.Since != "2026-07-01" || request.Where != `ContactStatus=="ACTIVE"` || request.Order != "Name DESC" {
		t.Fatalf("unexpected request: %+v", request)
	}
	wantIDs := []string{commandContactID, "88192a99-cbc5-4a66-bf1a-2f9fea2d36d0"}
	if len(request.ContactIDs) != len(wantIDs) || request.ContactIDs[0] != wantIDs[0] || request.ContactIDs[1] != wantIDs[1] {
		t.Fatalf("unexpected contact IDs: %v", request.ContactIDs)
	}
	output := stdout.String()
	if !strings.Contains(output, `"summary": "1 contact"`) || !strings.Contains(output, `"contactId": "`+commandContactID+`"`) || !strings.Contains(output, `"cmd": "xero contacts list --json"`) {
		t.Fatalf("unexpected JSON output: %s", output)
	}
	if strings.Contains(output, "bankAccount") || strings.Contains(output, "taxNumber") || strings.Contains(output, "balances") {
		t.Fatalf("sensitive fields leaked into output: %s", output)
	}
}

func TestContactsListHumanOutputUsesDefaultPhone(t *testing.T) {
	tempDir := t.TempDir()
	configPath := tempDir + "/config.json"
	prepareConfig(t, configPath)
	prepareSession(t, tempDir+"/session.json")
	contactClient := &fakeContactClient{listResult: xeroapi.ContactListResult{Contacts: []xeroapi.Contact{{
		ContactID:     commandContactID,
		Name:          "Acme",
		EmailAddress:  "acme@example.invalid",
		ContactStatus: "ACTIVE",
		Phones: []xeroapi.ContactPhone{
			{PhoneType: "MOBILE", PhoneNumber: "222"},
			{PhoneType: "DEFAULT", PhoneNumber: "111"},
		},
		IsCustomer: true,
		IsSupplier: false,
		UpdatedAt:  "2026-07-11T01:02:03Z",
	}}}}
	deps, stdout, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, &fakeLister{}, false)
	deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient { return contactClient }

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "contacts", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute contacts list: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "ID") || !strings.Contains(output, "NAME") || !strings.Contains(output, commandContactID) || !strings.Contains(output, "Acme") || !strings.Contains(output, "111") || strings.Contains(output, "222") || !strings.Contains(output, "1 contact") || !strings.Contains(output, "Next: show (xero contacts list --json)") {
		t.Fatalf("unexpected human output: %s", output)
	}
	if contactClient.listRequest.Page != 1 || contactClient.listRequest.Order != "" {
		t.Fatalf("expected bounded page and omitted order, got %+v", contactClient.listRequest)
	}
}

func TestContactsListQuietEmitsRawArray(t *testing.T) {
	tempDir := t.TempDir()
	configPath := tempDir + "/config.json"
	prepareConfig(t, configPath)
	prepareSession(t, tempDir+"/session.json")
	contactClient := &fakeContactClient{listResult: xeroapi.ContactListResult{Contacts: []xeroapi.Contact{{ContactID: commandContactID, Name: "Acme", Phones: []xeroapi.ContactPhone{}}}}}
	deps, stdout, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, &fakeLister{}, false)
	deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient { return contactClient }

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "contacts", "list", "--quiet"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute contacts list: %v", err)
	}
	output := stdout.String()
	if !strings.HasPrefix(output, "[") || strings.Contains(output, `"ok"`) || strings.Contains(output, `"summary"`) {
		t.Fatalf("expected raw array, got %s", output)
	}
}

func TestContactsListValidationRunsBeforeRuntime(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "empty search", args: []string{"--search", " "}},
		{name: "bad contact ID", args: []string{"--contact-id", "bad"}},
		{name: "empty contact ID", args: []string{"--contact-id", ""}},
		{name: "zero page", args: []string{"--page", "0"}},
		{name: "negative page", args: []string{"--page", "-1"}},
		{name: "zero page size", args: []string{"--page-size", "0"}},
		{name: "empty where", args: []string{"--where", " "}},
		{name: "empty order", args: []string{"--order", " "}},
		{name: "invalid order", args: []string{"--order", "Name sideways"}},
		{name: "empty since", args: []string{"--since", " "}},
		{name: "invalid since", args: []string{"--since", "07/11/2026"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := tempDir + "/config.json"
			prepareConfig(t, configPath)
			prepareSession(t, tempDir+"/session.json")
			contactClient := &fakeContactClient{}
			factoryCalls := 0
			deps, _, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, &fakeLister{}, false)
			deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient {
				factoryCalls++
				return contactClient
			}
			cmd := commands.NewRootCommand(deps)
			args := []string{"--config", configPath, "contacts", "list"}
			args = append(args, test.args...)
			cmd.SetArgs(args)
			err := cmd.Execute()
			if clierrors.KindOf(err) != clierrors.KindValidation {
				t.Fatalf("expected validation error, got %v", err)
			}
			if factoryCalls != 0 || contactClient.listCalls != 0 {
				t.Fatalf("expected no runtime/client work, factory=%d calls=%d", factoryCalls, contactClient.listCalls)
			}
		})
	}
}

func TestContactsParentRequiresExplicitListAndRejectsArguments(t *testing.T) {
	tempDir := t.TempDir()
	configPath := tempDir + "/config.json"
	prepareConfig(t, configPath)
	prepareSession(t, tempDir+"/session.json")
	contactClient := &fakeContactClient{}
	deps, stdout, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, &fakeLister{}, false)
	deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient { return contactClient }

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "contacts"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute contacts parent: %v", err)
	}
	if contactClient.listCalls != 0 || !strings.Contains(stdout.String(), "Available Commands") || !strings.Contains(stdout.String(), "list") {
		t.Fatalf("expected parent help without listing, output=%q calls=%d", stdout.String(), contactClient.listCalls)
	}

	cmd = commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "contacts", "list", "unexpected"})
	err := cmd.Execute()
	if err == nil || contactClient.listCalls != 0 {
		t.Fatalf("expected positional argument rejection without client call, err=%v calls=%d", err, contactClient.listCalls)
	}
}

func TestContactsListEmptyResultIsSuccessful(t *testing.T) {
	tempDir := t.TempDir()
	configPath := tempDir + "/config.json"
	prepareConfig(t, configPath)
	prepareSession(t, tempDir+"/session.json")
	contactClient := &fakeContactClient{listResult: xeroapi.ContactListResult{Contacts: []xeroapi.Contact{}}}
	deps, stdout, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, &fakeLister{}, false)
	deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient { return contactClient }

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "contacts", "list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute contacts list: %v", err)
	}
	if !strings.Contains(stdout.String(), `"data": []`) || !strings.Contains(stdout.String(), `"summary": "0 contacts"`) {
		t.Fatalf("unexpected empty output: %s", stdout.String())
	}
}

func TestContactsCreateFlagModeMapsPhoneAndEmitsJSON(t *testing.T) {
	tempDir := t.TempDir()
	configPath := tempDir + "/config.json"
	prepareConfig(t, configPath)
	prepareSession(t, tempDir+"/session.json")
	client := &fakeContactClient{mutationResult: xeroapi.ContactMutationResult{
		ContactID: commandContactID,
		Name:      "Acme Corp",
		Status:    "ACTIVE",
	}}
	deps, stdout, stderr := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, &fakeLister{}, false)
	deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient { return client }

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "contacts", "create", "--name", "Acme Corp", "--email", "acme@example.invalid", "--phone", "+1234567890", "--idempotency-key", "contact-create-key", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute contacts create: %v", err)
	}
	if stderr.Len() != 0 || client.createCalls != 1 {
		t.Fatalf("unexpected stderr/calls: stderr=%q calls=%d", stderr.String(), client.createCalls)
	}
	request := client.createRequest
	if request.TenantID != "tenant-1" || request.IdempotencyKey != "contact-create-key" || request.Contact.Name == nil || *request.Contact.Name != "Acme Corp" || request.Contact.EmailAddress == nil || *request.Contact.EmailAddress != "acme@example.invalid" {
		t.Fatalf("unexpected create request: %+v", request)
	}
	if request.Contact.ContactID != "" || request.Contact.ContactStatus != nil || request.Contact.Phones == nil || len(*request.Contact.Phones) != 1 || (*request.Contact.Phones)[0].PhoneType != "DEFAULT" || (*request.Contact.Phones)[0].PhoneNumber != "+1234567890" {
		t.Fatalf("unexpected create identity/phone mapping: %+v", request.Contact)
	}
	output := stdout.String()
	if !strings.Contains(output, `"operation": "created"`) || !strings.Contains(output, `"idempotencyKey": "contact-create-key"`) || !strings.Contains(output, `"cmd": "xero contacts list --contact-id `+commandContactID+` --include-archived --json"`) {
		t.Fatalf("unexpected JSON output: %s", output)
	}
}

func TestContactsCreateFileModeUsesStrictInputAndQuietOutput(t *testing.T) {
	tempDir := t.TempDir()
	configPath := tempDir + "/config.json"
	prepareConfig(t, configPath)
	prepareSession(t, tempDir+"/session.json")
	inputPath := tempDir + "/contact.json"
	if err := os.WriteFile(inputPath, []byte(`{"name":"File Contact","contactNumber":"CRM-1","accountNumber":"AC-1","firstName":"Alex","lastName":"Morgan","companyNumber":"COMP-1","emailAddress":"file@example.invalid","phone":"111"}`), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	client := &fakeContactClient{mutationResult: xeroapi.ContactMutationResult{ContactID: commandContactID, Name: "File Contact", Status: "ACTIVE"}}
	deps, stdout, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, &fakeLister{}, false)
	deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient { return client }
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "contacts", "create", "--file", inputPath, "--quiet"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute file create: %v", err)
	}
	if client.createRequest.Contact.ContactNumber == nil || *client.createRequest.Contact.ContactNumber != "CRM-1" || client.createRequest.Contact.AccountNumber == nil || *client.createRequest.Contact.AccountNumber != "AC-1" || len(client.createRequest.IdempotencyKey) != 64 {
		t.Fatalf("unexpected file request: %+v", client.createRequest)
	}
	if !strings.Contains(stdout.String(), `"contactId": "`+commandContactID+`"`) || strings.Contains(stdout.String(), `"ok"`) || strings.Contains(stdout.String(), `"summary"`) {
		t.Fatalf("unexpected quiet output: %s", stdout.String())
	}
}

func TestContactsUpdateFlagModePreservesExplicitEmptyAndOmission(t *testing.T) {
	tempDir := t.TempDir()
	configPath := tempDir + "/config.json"
	prepareConfig(t, configPath)
	prepareSession(t, tempDir+"/session.json")
	client := &fakeContactClient{mutationResult: xeroapi.ContactMutationResult{ContactID: commandContactID, Name: "Acme", Status: "ACTIVE"}}
	deps, stdout, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, &fakeLister{}, false)
	deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient { return client }
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "contacts", "update", "--contact-id", strings.ToUpper(commandContactID), "--email", "", "--first-name", "", "--idempotency-key", "update-key"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute contacts update: %v", err)
	}
	request := client.updateRequest
	if request.ContactID != commandContactID || request.Contact.ContactID != commandContactID || request.IdempotencyKey != "update-key" {
		t.Fatalf("identity was not injected: %+v", request)
	}
	if request.Contact.EmailAddress == nil || *request.Contact.EmailAddress != "" || request.Contact.FirstName == nil || *request.Contact.FirstName != "" {
		t.Fatalf("explicit empty values were not preserved: %+v", request.Contact)
	}
	if request.Contact.Name != nil || request.Contact.ContactNumber != nil || request.Contact.ContactStatus != nil || request.Contact.Phones != nil {
		t.Fatalf("omitted/unsupported fields became present: %+v", request.Contact)
	}
	if got := stdout.String(); got != "Updated contact Acme ("+commandContactID+") for tenant tenant-1 (ACTIVE)\nIdempotency key: update-key\n" {
		t.Fatalf("unexpected human output: %q", got)
	}
}

func TestContactsUpdateFileModeArchivesOnlyWithConfirmation(t *testing.T) {
	tempDir := t.TempDir()
	configPath := tempDir + "/config.json"
	prepareConfig(t, configPath)
	prepareSession(t, tempDir+"/session.json")
	inputPath := tempDir + "/archive.json"
	if err := os.WriteFile(inputPath, []byte(`{"contactId":"`+commandContactID+`","contactStatus":"ARCHIVED"}`), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	client := &fakeContactClient{mutationResult: xeroapi.ContactMutationResult{ContactID: commandContactID, Name: "Acme", Status: "ARCHIVED"}}
	deps, _, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, &fakeLister{}, false)
	deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient { return client }

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "contacts", "update", "--file", inputPath})
	if err := cmd.Execute(); clierrors.KindOf(err) != clierrors.KindValidation || client.updateCalls != 0 {
		t.Fatalf("expected unconfirmed archive rejection, got err=%v calls=%d", err, client.updateCalls)
	}

	cmd = commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "contacts", "update", "--file", inputPath, "--confirm-archive", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute confirmed archive: %v", err)
	}
	if client.updateCalls != 1 || client.updateRequest.Contact.ContactStatus == nil || *client.updateRequest.Contact.ContactStatus != "ARCHIVED" {
		t.Fatalf("unexpected archive request: calls=%d request=%+v", client.updateCalls, client.updateRequest)
	}
}

func TestContactsUpdateFileModeInjectsNormalizedFileIdentity(t *testing.T) {
	tempDir := t.TempDir()
	configPath := tempDir + "/config.json"
	prepareConfig(t, configPath)
	prepareSession(t, tempDir+"/session.json")
	inputPath := tempDir + "/update.json"
	if err := os.WriteFile(inputPath, []byte(`{"contactId":"`+strings.ToUpper(commandContactID)+`","emailAddress":""}`), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	client := &fakeContactClient{mutationResult: xeroapi.ContactMutationResult{ContactID: commandContactID, Name: "Acme", Status: "ACTIVE"}}
	deps, _, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, &fakeLister{}, false)
	deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient { return client }

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "contacts", "update", "--file", inputPath, "--idempotency-key", "file-update-key", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute file update: %v", err)
	}
	request := client.updateRequest
	if request.ContactID != commandContactID || request.Contact.ContactID != commandContactID {
		t.Fatalf("file identity was not normalized and injected consistently: %+v", request)
	}
	if request.Contact.EmailAddress == nil || *request.Contact.EmailAddress != "" || request.Contact.Name != nil {
		t.Fatalf("file update presence was not preserved: %+v", request.Contact)
	}
}

func TestContactMutationModeAndArchiveValidationRunBeforeRuntime(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "create missing mode", args: []string{"contacts", "create"}},
		{name: "create mixed file and flag", args: []string{"contacts", "create", "--file", "ignored.json", "--name", "Acme"}},
		{name: "update missing ID", args: []string{"contacts", "update", "--name", "Acme"}},
		{name: "update no changes", args: []string{"contacts", "update", "--contact-id", commandContactID}},
		{name: "update mixed file and ID", args: []string{"contacts", "update", "--file", "ignored.json", "--contact-id", commandContactID}},
		{name: "archive without confirmation", args: []string{"contacts", "update", "--contact-id", commandContactID, "--status", "ARCHIVED"}},
		{name: "archive with extra field", args: []string{"contacts", "update", "--contact-id", commandContactID, "--status", "ARCHIVED", "--name", "Acme", "--confirm-archive"}},
		{name: "confirm active", args: []string{"contacts", "update", "--contact-id", commandContactID, "--status", "ACTIVE", "--confirm-archive"}},
		{name: "GDPR request", args: []string{"contacts", "update", "--contact-id", commandContactID, "--status", "GDPRREQUEST"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factoryCalls := 0
			deps, _, _ := testDependencies(t.TempDir()+"/missing.json", &fakeStore{}, &fakeLister{}, false)
			deps.NewContactClient = func(appconfig.Settings) xeroapi.ContactClient {
				factoryCalls++
				return &fakeContactClient{}
			}
			cmd := commands.NewRootCommand(deps)
			cmd.SetArgs(test.args)
			err := cmd.Execute()
			if clierrors.KindOf(err) != clierrors.KindValidation || factoryCalls != 0 {
				t.Fatalf("expected pre-runtime validation, got err=%v factory=%d", err, factoryCalls)
			}
		})
	}
}

func TestContactsUpdateRejectsPhoneFlag(t *testing.T) {
	deps, _, _ := testDependencies(t.TempDir()+"/missing.json", &fakeStore{}, &fakeLister{}, false)
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"contacts", "update", "--contact-id", commandContactID, "--phone", "111"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --phone") {
		t.Fatalf("expected update phone rejection, got %v", err)
	}
}
