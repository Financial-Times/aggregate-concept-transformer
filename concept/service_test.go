package concept

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"

	"github.com/Financial-Times/cm-graph-ontology/v2/transform"

	"github.com/Financial-Times/aggregate-concept-transformer/concordances"
	"github.com/Financial-Times/aggregate-concept-transformer/sqs"
)

const (
	payload = `{
		"events": [
			{
				"conceptType": "Person",
				"conceptUUID": "28090964-9997-4bc2-9638-7a11135aaff9",
				"aggregateHash": "1234567890",
				"eventDetails": {
					"type": "Concept Updated"
				}
			},
			{
				"conceptType": "Person",
				"conceptUUID": "34a571fb-d779-4610-a7ba-2e127676db4d",
				"aggregateHash": "1234567890",
				"eventDetails": {
					"type": "Concept Updated"
				}
			},
			{
				"conceptType": "Person",
				"conceptUUID": "28090964-9997-4bc2-9638-7a11135aaff9",
				"aggregateHash": "1234567890",
				"eventDetails": {
					"type":  "Concordance Added",
					"oldID": "34a571fb-d779-4610-a7ba-2e127676db4d",
					"newID": "28090964-9997-4bc2-9638-7a11135aaff9"
				}
			}
		],
		"updatedIDs": [
			"28090964-9997-4bc2-9638-7a11135aaff9",
			"34a571fb-d779-4610-a7ba-2e127676db4d"
		]
	 }`
	membershipPayload = `{
		"events": [
			{
				"conceptType": "Membership",
				"conceptUUID": "ce922022-8114-11e8-8f42-da24cd01f044",
				"aggregateHash": "9876543210",
				"eventDetails": {
					"type": "Concept Updated"
				}
			}
		],
		"updatedIDs": [
			"ce922022-8114-11e8-8f42-da24cd01f044"
		]
	}`
	emptyPayload = `{
    			"updatedIDs": []
		 }`
	esUrl            = "concept-rw-elasticsearch"
	neo4jUrl         = "concepts-rw-neo4j"
	varnishPurgerUrl = "varnish-purger"
	inceptionDate    = "2000-01-01"
)

func TestNewService(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	assert.Equal(t, 8, len(svc.Healthchecks()))
}

func TestAggregateService_ListenForNotifications(t *testing.T) {
	svc, _, mockSqsClient, _, _, _, _ := setupTestService(200, payload)
	mockSqsClient.On("ListenAndServeQueue").Return([]sqs.ConceptUpdate{})
	go svc.ListenForNotifications(context.Background(), 1)
	time.Sleep(2 * time.Second)
	assert.Equal(t, 0, len(mockSqsClient.Queue()))
}

func TestAggregateService_ListenForNotifications_ProcessNoneIfNotHealthy(t *testing.T) {
	svc, _, mockSqsClient, _, _, fb, _ := setupTestService(200, payload)
	mockSqsClient.On("ListenAndServeQueue").Return([]sqs.ConceptUpdate{})
	fb <- false
	for len(fb) > 0 {
		time.Sleep(100 * time.Nanosecond)
	}
	time.Sleep(10 * time.Millisecond) // I hate waiting :(
	go svc.ListenForNotifications(context.Background(), 1)
	time.Sleep(2 * time.Second)
	mockSqsClient.AssertNotCalled(t, "ListenAndServeQueue")
	assert.Equal(t, 1, len(mockSqsClient.Queue()))
}

func TestAggregateService_ListenForNotifications_ProcessConceptNotInS3(t *testing.T) {
	svc, s3mock, mockSqsClient, _, _, _, _ := setupTestService(200, payload)
	mockSqsClient.On("ListenAndServeQueue").Return([]sqs.ConceptUpdate{})
	var receiptHandle = "1"
	var nonExistingConcept = "99247059-04ec-3abb-8693-a0b8951fdkor"
	mockSqsClient.conceptsQueue[receiptHandle] = nonExistingConcept
	go svc.ListenForNotifications(context.Background(), 1)
	time.Sleep(500 * time.Microsecond)
	hasIt, _, _, err := s3mock.GetConceptAndTransactionID(context.Background(), "", nonExistingConcept)
	assert.Equal(t, hasIt, false)
	assert.NoError(t, err)
	err = mockSqsClient.RemoveMessageFromQueue(context.Background(), &receiptHandle)
	assert.Equal(t, 0, len(mockSqsClient.Queue()))
	assert.NoError(t, err)
}

func TestAggregateService_ListenForNotifications_CannotProcessRemoveMessageNotPresentOnQueue(t *testing.T) {
	svc, _, mockSqsClient, _, _, _, _ := setupTestService(200, payload)
	mockSqsClient.On("ListenAndServeQueue").Return([]sqs.ConceptUpdate{})
	var receiptHandle = "2"
	go svc.ListenForNotifications(context.Background(), 1)
	err := mockSqsClient.RemoveMessageFromQueue(context.Background(), &receiptHandle)
	assert.Error(t, err)
	assert.Equal(t, "Receipt handle not present on conceptsQueue", err.Error())
}

func TestAggregateService_ProcessConceptUpdate_ContextTimeout(t *testing.T) {

	svc, s3mock, _, _, _, _, _ := setupTestServiceWithTimeout(200, payload, time.Millisecond*10)
	s3mock.callsMocked = true
	s3mock.On("GetConceptAndTransactionID", "fb9fd611-0822-4283-b1b2-e691804ec5d5").Return(false, transform.OldConcept{}, "", nil).After(time.Second * 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	update := sqs.ConceptUpdate{
		UUID: "fb9fd611-0822-4283-b1b2-e691804ec5d5",
	}
	err := svc.processConceptUpdate(ctx, update)
	assert.EqualError(t, err, "context deadline exceeded")
}

func TestAggregateService_GetConcordedConcept_NoConcordance(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)

	c, tid, err := getConceptFromService(svc, context.Background(), "99247059-04ec-3abb-8693-a0b8951fdcab", "")
	assert.NoError(t, err)
	assert.Equal(t, "tid_123", tid)
	assert.Equal(t, "Test Concept", c.PrefLabel)
	assert.Equal(t, "Mr", c.Salutation)
	assert.Equal(t, 2018, c.BirthYear)
}

func TestAggregateService_GetConcordedConcept_CancelContext(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, tid, err := getConceptFromService(svc, ctx, "99247059-04ec-3abb-8693-a0b8951fdcab", "")
	assert.EqualError(t, err, "context canceled")
	assert.Equal(t, "", tid)
}

func TestAggregateService_GetConcordedConcept_Location(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:  "f8024a12-2d71-4f0e-996d-bcbc07df3921",
		PrefLabel: "Paris",
		Type:      "Location",
		Aliases:   []string{"Paris", "Paris, Texas"},
		ScopeNote: "Paris, Texas",
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "900dd202-fccc-3280-b053-d46c234dcbe2",
				PrefLabel:      "Paris, Texas",
				Authority:      "TME",
				AuthorityValue: "UGFyaXMsIFRleGFz-R0w=",
				Type:           "Location",
			},
			{
				UUID:           "f8024a12-2d71-4f0e-996d-bcbc07df3921",
				PrefLabel:      "Paris",
				Authority:      "Smartlogic",
				AuthorityValue: "f8024a12-2d71-4f0e-996d-bcbc07df3921",
				Type:           "Location",
			},
		},
	}
	c, tid, err := getConceptFromService(svc, context.Background(), "f8024a12-2d71-4f0e-996d-bcbc07df3921", "")
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_999", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_ManagedLocationCountry(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:  "c78371f2-1f55-4099-ae63-f44e44bb2af8",
		PrefLabel: "France",
		Type:      "Location",
		Aliases:   []string{"France", "French Republic"},
		ScopeNote: "French Republic",
		ISO31661:  "FR",
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "FR_TME_UUID",
				PrefLabel:      "French Republic",
				Authority:      "TME",
				AuthorityValue: "FR_TME_AUTH_VALUE",
				Type:           "Location",
			},
			{
				UUID:           "c78371f2-1f55-4099-ae63-f44e44bb2af8",
				PrefLabel:      "France",
				Authority:      "ManagedLocation",
				AuthorityValue: "c78371f2-1f55-4099-ae63-f44e44bb2af8",
				Type:           "Location",
				ISO31661:       "FR",
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "c78371f2-1f55-4099-ae63-f44e44bb2af8", "")
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_112", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_SmartlogicCountry(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:  "cc3bf637-9288-499b-9221-bb6e8e003f03",
		PrefLabel: "Belgium",
		Type:      "Location",
		Aliases:   []string{"Belgium", "Kingdom of Belgium", "Royaume de Belgique"},
		ScopeNote: "Royaume de Belgique",
		ISO31661:  "BE",
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "BE_ML_UUID",
				PrefLabel:      "Kingdom of Belgium",
				Authority:      "ManagedLocation",
				AuthorityValue: "BE_ML_UUID",
				Type:           "Location",
				ISO31661:       "BE",
			},
			{
				UUID:           "BE_TME_UUID",
				PrefLabel:      "Royaume de Belgique",
				Authority:      "TME",
				AuthorityValue: "BE_TME_AUTH_VALUE",
				Type:           "Location",
			},
			{
				UUID:           "cc3bf637-9288-499b-9221-bb6e8e003f03",
				PrefLabel:      "Belgium",
				Authority:      "Smartlogic",
				AuthorityValue: "cc3bf637-9288-499b-9221-bb6e8e003f03",
				Type:           "Location",
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "cc3bf637-9288-499b-9221-bb6e8e003f03", "")
	sort.Strings(expectedConcept.Aliases)
	sort.Slice(c.SourceRepresentations, func(i, j int) bool {
		return c.SourceRepresentations[i].UUID < c.SourceRepresentations[j].UUID
	})
	sort.Slice(expectedConcept.SourceRepresentations, func(i, j int) bool {
		return expectedConcept.SourceRepresentations[i].UUID < expectedConcept.SourceRepresentations[j].UUID
	})

	assert.NoError(t, err)
	assert.Equal(t, "tid_358", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetExternalConcordedConcept(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:  "f3633e04-2ee3-48ce-8081-37734dab3fdc",
		PrefLabel: "Capital Flows",
		Type:      "TestConcept",
		Aliases:   []string{"Capital Flows"},
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "f3633e04-2ee3-48ce-8081-37734dab3fdc",
				PrefLabel:      "Capital Flows",
				Authority:      "929da855-c1ba-4576-89c1-5c3ec9e4c6ef",
				AuthorityValue: "f3633e04-2ee3-48ce-8081-37734dab3fdc",
				Type:           "TestConcept",
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "929da855-c1ba-4576-89c1-5c3ec9e4c6ef-f3633e04-2ee3-48ce-8081-37734dab3fdc", "")
	sort.Strings(expectedConcept.Aliases)
	sort.Slice(c.SourceRepresentations, func(i, j int) bool {
		return c.SourceRepresentations[i].UUID < c.SourceRepresentations[j].UUID
	})
	sort.Slice(expectedConcept.SourceRepresentations, func(i, j int) bool {
		return expectedConcept.SourceRepresentations[i].UUID < expectedConcept.SourceRepresentations[j].UUID
	})

	assert.NoError(t, err)
	assert.Equal(t, "tid_358", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_TMEConcordance(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:        "28090964-9997-4bc2-9638-7a11135aaff9",
		PrefLabel:       "Root Concept",
		Type:            "TestConcept",
		Aliases:         []string{"TME Concept", "Root Concept"},
		EmailAddress:    "person123@ft.com",
		FacebookPage:    "facebook/smartlogicPerson",
		TwitterHandle:   "@FtSmartlogicPerson",
		ScopeNote:       "This note is in scope",
		ShortLabel:      "Concept",
		InceptionDate:   "2002-06-01",
		TerminationDate: "2011-11-29",
		FigiCode:        "BBG000Y1HJT8",
		IssuedBy:        "613b1f72-cc74-4d8f-9406-28fc91b82a2a",
		MembershipRoles: []transform.MembershipRole{
			{
				RoleUUID:        "ccdff192-4d6c-4539-bbe8-7e24e81ed49e",
				InceptionDate:   "2002-06-01",
				TerminationDate: "2011-11-29",
			},
		},
		OrganisationUUID: "a4528fc9-0615-4bfa-bc99-596ea1ddec28",
		PersonUUID:       "973509c1-5238-4c83-9a7d-89009e839ff8",
		IsDeprecated:     false,
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "34a571fb-d779-4610-a7ba-2e127676db4d",
				PrefLabel:      "TME Concept",
				Authority:      "TME",
				AuthorityValue: "TME-123",
				Type:           "TestConcept",
				IsDeprecated:   true,
			},
			{
				UUID:            "28090964-9997-4bc2-9638-7a11135aaff9",
				PrefLabel:       "Root Concept",
				Authority:       "Smartlogic",
				AuthorityValue:  "28090964-9997-4bc2-9638-7a11135aaff9",
				Type:            "TestConcept",
				FacebookPage:    "facebook/smartlogicPerson",
				TwitterHandle:   "@FtSmartlogicPerson",
				ScopeNote:       "This note is in scope",
				EmailAddress:    "person123@ft.com",
				ShortLabel:      "Concept",
				InceptionDate:   "2002-06-01",
				TerminationDate: "2011-11-29",
				FigiCode:        "BBG000Y1HJT8",
				IssuedBy:        "613b1f72-cc74-4d8f-9406-28fc91b82a2a",
				MembershipRoles: []transform.MembershipRole{
					{
						RoleUUID:        "ccdff192-4d6c-4539-bbe8-7e24e81ed49e",
						InceptionDate:   "2002-06-01",
						TerminationDate: "2011-11-29",
					},
				},
				OrganisationUUID: "a4528fc9-0615-4bfa-bc99-596ea1ddec28",
				PersonUUID:       "973509c1-5238-4c83-9a7d-89009e839ff8",
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "28090964-9997-4bc2-9638-7a11135aaff9", "")
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_456", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_DeprecatedSmartlogic(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:        "28090964-9997-4bc2-9638-7a11135aaf10",
		PrefLabel:       "Root Concept",
		Type:            "TestConcept",
		Aliases:         []string{"TME Concept", "Root Concept"},
		EmailAddress:    "person123@ft.com",
		FacebookPage:    "facebook/smartlogicPerson",
		TwitterHandle:   "@FtSmartlogicPerson",
		ScopeNote:       "This note is in scope",
		ShortLabel:      "Concept",
		InceptionDate:   "2002-06-01",
		TerminationDate: "2011-11-29",
		FigiCode:        "BBG000Y1HJT8",
		IssuedBy:        "613b1f72-cc74-4d8f-9406-28fc91b82a2a",
		MembershipRoles: []transform.MembershipRole{
			{
				RoleUUID:        "ccdff192-4d6c-4539-bbe8-7e24e81ed49e",
				InceptionDate:   "2002-06-01",
				TerminationDate: "2011-11-29",
			},
		},
		OrganisationUUID: "a4528fc9-0615-4bfa-bc99-596ea1ddec28",
		PersonUUID:       "973509c1-5238-4c83-9a7d-89009e839ff8",
		IsDeprecated:     true,
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "34a571fb-d779-4610-a7ba-2e127676db4e",
				PrefLabel:      "TME Concept",
				Authority:      "TME",
				AuthorityValue: "TME-123",
				Type:           "TestConcept",
				IsDeprecated:   false,
			},
			{
				UUID:            "28090964-9997-4bc2-9638-7a11135aaf10",
				PrefLabel:       "Root Concept",
				Authority:       "Smartlogic",
				AuthorityValue:  "28090964-9997-4bc2-9638-7a11135aaf10",
				Type:            "TestConcept",
				FacebookPage:    "facebook/smartlogicPerson",
				TwitterHandle:   "@FtSmartlogicPerson",
				ScopeNote:       "This note is in scope",
				EmailAddress:    "person123@ft.com",
				ShortLabel:      "Concept",
				InceptionDate:   "2002-06-01",
				TerminationDate: "2011-11-29",
				FigiCode:        "BBG000Y1HJT8",
				IssuedBy:        "613b1f72-cc74-4d8f-9406-28fc91b82a2a",
				MembershipRoles: []transform.MembershipRole{
					{
						RoleUUID:        "ccdff192-4d6c-4539-bbe8-7e24e81ed49e",
						InceptionDate:   "2002-06-01",
						TerminationDate: "2011-11-29",
					},
				},
				OrganisationUUID: "a4528fc9-0615-4bfa-bc99-596ea1ddec28",
				PersonUUID:       "973509c1-5238-4c83-9a7d-89009e839ff8",
				IsDeprecated:     true,
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "28090964-9997-4bc2-9638-7a11135aaf10", "")
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_456", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_SupersededConcept(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:        "28090964-9997-4bc2-9638-7a11135aaf11",
		PrefLabel:       "Root Concept",
		Type:            "TestConcept",
		Aliases:         []string{"Root Concept"},
		EmailAddress:    "person123@ft.com",
		FacebookPage:    "facebook/smartlogicPerson",
		TwitterHandle:   "@FtSmartlogicPerson",
		ScopeNote:       "This note is in scope",
		ShortLabel:      "Concept",
		InceptionDate:   "2002-06-01",
		TerminationDate: "2011-11-29",
		FigiCode:        "BBG000Y1HJT8",
		IssuedBy:        "613b1f72-cc74-4d8f-9406-28fc91b82a2a",
		SupersededByUUIDs: []string{
			"28090964-9997-4bc2-9638-7a11135aaff9",
		},
		MembershipRoles: []transform.MembershipRole{
			{
				RoleUUID:        "ccdff192-4d6c-4539-bbe8-7e24e81ed49e",
				InceptionDate:   "2002-06-01",
				TerminationDate: "2011-11-29",
			},
		},
		OrganisationUUID: "a4528fc9-0615-4bfa-bc99-596ea1ddec28",
		PersonUUID:       "973509c1-5238-4c83-9a7d-89009e839ff8",
		IsDeprecated:     true,
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:            "28090964-9997-4bc2-9638-7a11135aaf11",
				PrefLabel:       "Root Concept",
				Authority:       "Smartlogic",
				AuthorityValue:  "28090964-9997-4bc2-9638-7a11135aaf11",
				Type:            "TestConcept",
				FacebookPage:    "facebook/smartlogicPerson",
				TwitterHandle:   "@FtSmartlogicPerson",
				ScopeNote:       "This note is in scope",
				EmailAddress:    "person123@ft.com",
				ShortLabel:      "Concept",
				InceptionDate:   "2002-06-01",
				TerminationDate: "2011-11-29",
				FigiCode:        "BBG000Y1HJT8",
				IssuedBy:        "613b1f72-cc74-4d8f-9406-28fc91b82a2a",
				SupersededByUUIDs: []string{
					"28090964-9997-4bc2-9638-7a11135aaff9",
				},
				MembershipRoles: []transform.MembershipRole{
					{
						RoleUUID:        "ccdff192-4d6c-4539-bbe8-7e24e81ed49e",
						InceptionDate:   "2002-06-01",
						TerminationDate: "2011-11-29",
					},
				},
				OrganisationUUID: "a4528fc9-0615-4bfa-bc99-596ea1ddec28",
				PersonUUID:       "973509c1-5238-4c83-9a7d-89009e839ff8",
				IsDeprecated:     true,
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "28090964-9997-4bc2-9638-7a11135aaf11", "")
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_456", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_ConceptWithRelationships(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:       "781bb463-dc53-4d3e-9d49-c48dc4cf6d55",
		PrefLabel:      "Test FT Brand",
		Type:           "Brand",
		Aliases:        []string{"Test FT Brand"},
		ParentUUIDs:    []string{"ec467314-63cf-4976-a124-77175d10423d"},
		BroaderUUIDs:   []string{"575a2223-6307-4000-8882-935c27f4e8bb"},
		RelatedUUIDs:   []string{"b73e632c-9b8d-477d-bb45-aaf574bc015c"},
		DescriptionXML: "<body>The best brand</body>",
		Strapline:      "The Best Brand",
		ImageURL:       "localhost:8080/12345",
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "781bb463-dc53-4d3e-9d49-c48dc4cf6d55",
				PrefLabel:      "Test FT Brand",
				Authority:      "Smartlogic",
				AuthorityValue: "781bb463-dc53-4d3e-9d49-c48dc4cf6d55",
				Type:           "Brand",
				DescriptionXML: "<body>The best brand</body>",
				Strapline:      "The Best Brand",
				ImageURL:       "localhost:8080/12345",
				ParentUUIDs:    []string{"ec467314-63cf-4976-a124-77175d10423d"},
				BroaderUUIDs:   []string{"575a2223-6307-4000-8882-935c27f4e8bb"},
				RelatedUUIDs:   []string{"b73e632c-9b8d-477d-bb45-aaf574bc015c"},
				ImpliedByUUIDs: []string{"b5d7c6b5-db7d-4bce-9d6a-f62195571f92"},
				HasFocusUUIDs:  []string{"2e7429bd-7a84-41cb-a619-2c702893e359"},
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "781bb463-dc53-4d3e-9d49-c48dc4cf6d55", "")
	assert.NoError(t, err)
	assert.Equal(t, "tid_633", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_FinancialInstrument(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:     "6562674e-dbfa-4cb0-85b2-41b0948b7cc2",
		PrefLabel:    "Some random financial instrument",
		Type:         "FinancialInstrument",
		Aliases:      []string{"Some random financial instrument"},
		FigiCode:     "BBG000Y1HJT8",
		IssuedBy:     "4e484678-cf47-4168-b844-6adb47f8eb58",
		IsDeprecated: false,
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "6562674e-dbfa-4cb0-85b2-41b0948b7cc2",
				PrefLabel:      "Some random financial instrument",
				Authority:      "FACTSET",
				AuthorityValue: "B000BB-S",
				Type:           "FinancialInstrument",
				FigiCode:       "BBG000Y1HJT8",
				IssuedBy:       "4e484678-cf47-4168-b844-6adb47f8eb58",
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "6562674e-dbfa-4cb0-85b2-41b0948b7cc2", "")
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_630", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_Organisation(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:   "c28fa0b4-4245-11e8-842f-0ed5f89f718b",
		Type:       "PublicCompany",
		ProperName: "Strix Group Plc",
		PrefLabel:  "Strix Group Plc",
		ShortName:  "Strix Group",
		TradeNames: []string{"Strixy"},
		FormerNames: []string{
			"Castletown Thermostats",
			"Steam Plc",
		},
		Aliases: []string{
			"Strix Group Plc",
			"STRIX GROUP PLC",
			"Strix Group",
			"Castletown Thermostats",
			"Steam Plc",
		},
		CountryCode:            "GB",
		CountryOfRisk:          "GB",
		CountryOfIncorporation: "IM",
		CountryOfOperations:    "GB",
		PostalCode:             "IM9 2RG",
		YearFounded:            1951,
		EmailAddress:           "info@strix.com",
		LeiCode:                "213800KZEW5W6BZMNT62",
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "c28fa0b4-4245-11e8-842f-0ed5f89f718b",
				Type:           "PublicCompany",
				Authority:      "FACTSET",
				AuthorityValue: "B000BB-S",
				ProperName:     "Strix Group Plc",
				PrefLabel:      "Strix Group Plc",
				ShortName:      "Strix Group",
				TradeNames:     []string{"Strixy"},
				FormerNames: []string{
					"Castletown Thermostats",
					"Steam Plc",
				},
				Aliases: []string{
					"Strix Group Plc",
					"STRIX GROUP PLC",
					"Strix Group",
					"Castletown Thermostats",
					"Steam Plc",
				},
				CountryCode:                "GB",
				CountryOfRisk:              "GB",
				CountryOfIncorporation:     "IM",
				CountryOfOperations:        "GB",
				CountryOfRiskUUID:          "GB_UUID",
				CountryOfIncorporationUUID: "IM_UUID",
				CountryOfOperationsUUID:    "GB_UUID",
				PostalCode:                 "IM9 2RG",
				YearFounded:                1951,
				EmailAddress:               "info@strix.com",
				LeiCode:                    "213800KZEW5W6BZMNT62",
				ParentOrganisation:         "123",
			},
		},
	}
	c, tid, err := getConceptFromService(svc, context.Background(), "c28fa0b4-4245-11e8-842f-0ed5f89f718b", "")
	sort.Strings(expectedConcept.FormerNames)
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_631", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_PublicCompany(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:   "a141f50f-31d7-4f89-8143-eec971e54ba8",
		Type:       "PublicCompany",
		ProperName: "Strix Group Plc",
		PrefLabel:  "Test FT Concorded Organisation",
		ShortName:  "Strix Group",
		TradeNames: []string{"Strixy"},
		FormerNames: []string{
			"Castletown Thermostats",
			"Steam Plc",
		},
		Aliases: []string{
			"Strix Group Plc",
			"STRIX GROUP PLC",
			"Strix Group",
			"Castletown Thermostats",
			"Steam Plc",
			"Test FT Concorded Organisation",
		},
		CountryCode:            "GB",
		CountryOfRisk:          "GB",
		CountryOfIncorporation: "IM",
		CountryOfOperations:    "GB",
		PostalCode:             "IM9 2RG",
		YearFounded:            1951,
		EmailAddress:           "info@strix.com",
		LeiCode:                "213800KZEW5W6BZMNT62",
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "c28fa0b4-4245-11e8-842f-0ed5f89f718b",
				Type:           "PublicCompany",
				Authority:      "FACTSET",
				AuthorityValue: "B000BB-S",
				ProperName:     "Strix Group Plc",
				PrefLabel:      "Strix Group Plc",
				ShortName:      "Strix Group",
				TradeNames:     []string{"Strixy"},
				FormerNames: []string{
					"Castletown Thermostats",
					"Steam Plc",
				},
				Aliases: []string{
					"Strix Group Plc",
					"STRIX GROUP PLC",
					"Strix Group",
					"Castletown Thermostats",
					"Steam Plc",
				},
				CountryCode:                "GB",
				CountryOfRisk:              "GB",
				CountryOfIncorporation:     "IM",
				CountryOfOperations:        "GB",
				CountryOfRiskUUID:          "GB_UUID",
				CountryOfIncorporationUUID: "IM_UUID",
				CountryOfOperationsUUID:    "GB_UUID",
				PostalCode:                 "IM9 2RG",
				YearFounded:                1951,
				EmailAddress:               "info@strix.com",
				LeiCode:                    "213800KZEW5W6BZMNT62",
				ParentOrganisation:         "123",
			},
			{
				UUID:           "a141f50f-31d7-4f89-8143-eec971e54ba8",
				PrefLabel:      "Test FT Concorded Organisation",
				Authority:      "Smartlogic",
				AuthorityValue: "a141f50f-31d7-4f89-8143-eec971e54ba8",
				Type:           "Organisation",
			},
		},
	}
	c, tid, err := getConceptFromService(svc, context.Background(), "a141f50f-31d7-4f89-8143-eec971e54ba8", "")
	sort.Strings(expectedConcept.FormerNames)
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_636", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_PublicCompany_WithNAICSCodes(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:  "1b72a837-b6a6-4490-85e2-c174101bfa10",
		Type:      "PublicCompany",
		PrefLabel: "Apple, Inc.",
		Aliases: []string{
			"Apple, Inc.",
		},
		NAICSIndustryClassifications: []transform.NAICSIndustryClassification{
			{
				UUID: "25c3be2a-15e0-434e-aaa9-ca067e70ae11",
				Rank: 1,
			},
			{
				UUID: "c9cae3ee-a804-4001-8046-02e714e1fa5b",
				Rank: 2,
			},
			{
				UUID: "7f0134c1-606a-4d12-a455-661cc4b0bfac",
				Rank: 3,
			},
			{
				UUID: "c35b851c-d185-40bd-967c-2403084920b3",
				Rank: 4,
			},
		},
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "Organisation_WithNAICSCodes_Factset_UUID",
				Type:           "PublicCompany",
				Authority:      "FACTSET",
				AuthorityValue: "000C7F-E",
				PrefLabel:      "Apple, Inc.",
				NAICSIndustryClassifications: []transform.NAICSIndustryClassification{
					{
						UUID: "25c3be2a-15e0-434e-aaa9-ca067e70ae11",
						Rank: 1,
					},
					{
						UUID: "c9cae3ee-a804-4001-8046-02e714e1fa5b",
						Rank: 2,
					},
					{
						UUID: "7f0134c1-606a-4d12-a455-661cc4b0bfac",
						Rank: 3,
					},
					{
						UUID: "c35b851c-d185-40bd-967c-2403084920b3",
						Rank: 4,
					},
				},
			},
			{
				UUID:           "1b72a837-b6a6-4490-85e2-c174101bfa10",
				Type:           "PublicCompany",
				Authority:      "Smartlogic",
				AuthorityValue: "000C7F-E",
				PrefLabel:      "Apple, Inc.",
			},
		},
	}
	c, tid, err := getConceptFromService(svc, context.Background(), "1b72a837-b6a6-4490-85e2-c174101bfa10", "")
	sort.Strings(expectedConcept.FormerNames)
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_736", tid)
	naicsOps := cmpopts.SortSlices(func(l, r transform.NAICSIndustryClassification) bool {
		return l.Rank < r.Rank
	})
	if !cmp.Equal(expectedConcept, c, naicsOps) {
		diff := cmp.Diff(expectedConcept, c, naicsOps)
		t.Fatal(diff)
	}
}

func TestAggregateService_GetConcordedConcept_BoardRole(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:  "344fdb1d-0585-31f7-814f-b478e54dbe1f",
		PrefLabel: "Director/Board Member",
		Type:      "BoardRole",
		Aliases:   []string{"Director/Board Member"},
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "344fdb1d-0585-31f7-814f-b478e54dbe1f",
				PrefLabel:      "Director/Board Member",
				Authority:      "FACTSET",
				AuthorityValue: "BRD",
				Type:           "BoardRole",
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "344fdb1d-0585-31f7-814f-b478e54dbe1f", "")
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_631", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_LoneTME(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:  "99309d51-8969-4a1e-8346-d51f1981479b",
		PrefLabel: "Lone TME Concept",
		Type:      "Person",
		Aliases:   []string{"Lone TME Concept"},
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:           "99309d51-8969-4a1e-8346-d51f1981479b",
				PrefLabel:      "Lone TME Concept",
				Authority:      "TME",
				AuthorityValue: "TME-qwe",
				Type:           "Person",
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "99309d51-8969-4a1e-8346-d51f1981479b", "")
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_439", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_Memberships(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:         "87cda39a-e354-3dfb-b28a-b9a04887577b",
		PrefLabel:        "Independent Non-Executive Director",
		Type:             "Membership",
		Aliases:          []string{"Independent Non-Executive Director"},
		PersonUUID:       "d4050b35-45ac-3933-9fad-7720a0dce8df",
		OrganisationUUID: "064ce159-8835-3426-b456-c86d48de8511",
		InceptionDate:    "2002-06-01",
		TerminationDate:  "2011-11-30",
		MembershipRoles: []transform.MembershipRole{
			{

				RoleUUID:        "344fdb1d-0585-31f7-814f-b478e54dbe1f",
				InceptionDate:   "2002-06-01",
				TerminationDate: "2011-11-29",
			},
			{
				RoleUUID:        "abacb0e1-3f7e-334a-96b9-ed5da35f3251",
				InceptionDate:   "2011-07-26",
				TerminationDate: "2011-11-29",
			},
		},
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:             "87cda39a-e354-3dfb-b28a-b9a04887577b",
				PrefLabel:        "Independent Non-Executive Director",
				Authority:        "FACTSET",
				AuthorityValue:   "1000016",
				Type:             "Membership",
				PersonUUID:       "d4050b35-45ac-3933-9fad-7720a0dce8df",
				OrganisationUUID: "064ce159-8835-3426-b456-c86d48de8511",
				InceptionDate:    "2002-06-01",
				TerminationDate:  "2011-11-30",
				MembershipRoles: []transform.MembershipRole{
					{

						RoleUUID:        "344fdb1d-0585-31f7-814f-b478e54dbe1f",
						InceptionDate:   "2002-06-01",
						TerminationDate: "2011-11-29",
					},
					{
						RoleUUID:        "abacb0e1-3f7e-334a-96b9-ed5da35f3251",
						InceptionDate:   "2011-07-26",
						TerminationDate: "2011-11-29",
					},
				},
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "87cda39a-e354-3dfb-b28a-b9a04887577b", "")
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_632", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_GetConcordedConcept_IndustryClassification(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	expectedConcept := transform.OldAggregatedConcept{
		PrefUUID:           "acb19f07-bfd0-4301-a96f-ab5e5c20e533",
		PrefLabel:          "Newspaper, Periodical, Book, and Directory Publishers",
		Type:               "NAICSIndustryClassification",
		Aliases:            []string{"Newspaper, Periodical, Book, and Directory Publishers"},
		IndustryIdentifier: "5111",
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:               "acb19f07-bfd0-4301-a96f-ab5e5c20e533",
				PrefLabel:          "Newspaper, Periodical, Book, and Directory Publishers",
				IndustryIdentifier: "5111",
				Authority:          "Smartlogic",
				AuthorityValue:     "acb19f07-bfd0-4301-a96f-ab5e5c20e533",
				Type:               "NAICSIndustryClassification",
			},
		},
	}

	c, tid, err := getConceptFromService(svc, context.Background(), "acb19f07-bfd0-4301-a96f-ab5e5c20e533", "")
	sort.Strings(expectedConcept.Aliases)
	assert.NoError(t, err)
	assert.Equal(t, "tid_359", tid)
	assert.Equal(t, expectedConcept, c)
}

func TestAggregateService_ProcessMessage_Success(t *testing.T) {
	svc, _, _, eventQueue, _, _, _ := setupTestService(200, payload)
	err := svc.ProcessMessage(context.Background(), "28090964-9997-4bc2-9638-7a11135aaff9", "")
	mockWriter := svc.httpClient.(*mockHTTPClient)
	assert.Equal(t, []string{
		"concepts-rw-neo4j/test-concepts/28090964-9997-4bc2-9638-7a11135aaff9",
		"varnish-purger/purge?target=%2Fthings%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fconcepts%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fthings%2F34a571fb-d779-4610-a7ba-2e127676db4d" +
			"&target=%2Fconcepts%2F34a571fb-d779-4610-a7ba-2e127676db4d",
		"concept-rw-elasticsearch/test-concepts/28090964-9997-4bc2-9638-7a11135aaff9",
	}, mockWriter.called)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(eventQueue.eventList))
}

func TestAggregateService_ProcessMessage_FinancialInstrumentsNotSentToEs(t *testing.T) {
	svc, _, _, eventQueue, _, _, _ := setupTestService(200, payload)
	err := svc.ProcessMessage(context.Background(), "6562674e-dbfa-4cb0-85b2-41b0948b7cc2", "")
	mockWriter := svc.httpClient.(*mockHTTPClient)
	assert.Equal(t, []string{
		"concepts-rw-neo4j/financial-instruments/6562674e-dbfa-4cb0-85b2-41b0948b7cc2",
		"varnish-purger/purge?target=%2Fthings%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fconcepts%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fthings%2F34a571fb-d779-4610-a7ba-2e127676db4d" +
			"&target=%2Fconcepts%2F34a571fb-d779-4610-a7ba-2e127676db4d",
		"varnish-purger/purge?target=%2Fthings%2F4e484678-cf47-4168-b844-6adb47f8eb58" +
			"&target=%2Fconcepts%2F4e484678-cf47-4168-b844-6adb47f8eb58" +
			"&target=%2Forganisations%2F4e484678-cf47-4168-b844-6adb47f8eb58",
	}, mockWriter.called)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(eventQueue.eventList))
}

func TestAggregateService_ProcessMessage_MembershipRolesNotSentToEs(t *testing.T) {
	svc, _, _, eventQueue, _, _, _ := setupTestService(200, payload)
	err := svc.ProcessMessage(context.Background(), "01e284c2-7d77-4df6-8df7-57ec006194a4", "")
	mockWriter := svc.httpClient.(*mockHTTPClient)
	assert.Equal(t, []string{
		"concepts-rw-neo4j/membership-roles/01e284c2-7d77-4df6-8df7-57ec006194a4",
		"varnish-purger/purge?target=%2Fthings%2F28090964-9997-4bc2-9638-7a11135aaff9&target=%2" +
			"Fconcepts%2F28090964-9997-4bc2-9638-7a11135aaff9&target=%2Fthings%2F" +
			"34a571fb-d779-4610-a7ba-2e127676db4d&target=%2Fconcepts%2F34a571fb-d779-4610-a7ba-2e127676db4d",
	}, mockWriter.called)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(eventQueue.eventList))
}

func TestAggregateService_ProcessMessage_BoardRolesNotSentToEs(t *testing.T) {
	svc, _, _, eventQueue, _, _, _ := setupTestService(200, payload)
	err := svc.ProcessMessage(context.Background(), "344fdb1d-0585-31f7-814f-b478e54dbe1f", "")
	mockWriter := svc.httpClient.(*mockHTTPClient)
	assert.Equal(t, []string{
		"concepts-rw-neo4j/membership-roles/344fdb1d-0585-31f7-814f-b478e54dbe1f",
		"varnish-purger/purge?target=%2Fthings%2F28090964-9997-4bc2-9638-7a11135aaff9&target=%2" +
			"Fconcepts%2F28090964-9997-4bc2-9638-7a11135aaff9&target=%2Fthings%2F" +
			"34a571fb-d779-4610-a7ba-2e127676db4d&target=%2Fconcepts%2F34a571fb-d779-4610-a7ba-2e127676db4d",
	}, mockWriter.called)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(eventQueue.eventList))
}

func TestAggregateService_ProcessMessage_FactsetMembershipNotSentToEs(t *testing.T) {
	svc, _, _, eventQueue, _, _, _ := setupTestService(200, payload)
	err := svc.ProcessMessage(context.Background(), "f784be91-601a-42db-ac57-e1d5da8b4866", "")
	mockWriter := svc.httpClient.(*mockHTTPClient)
	assert.Equal(t, []string{
		"concepts-rw-neo4j/memberships/f784be91-601a-42db-ac57-e1d5da8b4866",
		"varnish-purger/purge?target=%2Fthings%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fconcepts%2F28090964-9997-4bc2-9638-7a11135aaff9&target=%2Fthings" +
			"%2F34a571fb-d779-4610-a7ba-2e127676db4d&target=%2Fconcepts%2F34a571fb-d779-4610-a7ba-2e127676db4d",
		"varnish-purger/purge?target=%2Fthings%2F99309d51-8969-4a1e-8346-d51f1981479b&target=%2F" +
			"concepts%2F99309d51-8969-4a1e-8346-d51f1981479b&target=%2Fpeople%2F99309d51-8969-4a1e-8346-d51f1981479b",
	}, mockWriter.called)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(eventQueue.eventList))
}

func TestAggregateService_ProcessMessage_SmartlogicMembershipSentToEs(t *testing.T) {
	svc, _, _, eventQueue, _, _, _ := setupTestService(200, payload)
	err := svc.ProcessMessage(context.Background(), "ddacda04-b7cd-4d2e-86b1-7dfef0ff56a2", "")
	mockWriter := svc.httpClient.(*mockHTTPClient)
	assert.Equal(t, []string{
		"concepts-rw-neo4j/memberships/ddacda04-b7cd-4d2e-86b1-7dfef0ff56a2",
		"varnish-purger/purge?target=%2Fthings%2F28090964-9997-4bc2-9638-7a11135aaff9&target=%2Fconcepts" +
			"%2F28090964-9997-4bc2-9638-7a11135aaff9&target=%2Fthings%2F34a571fb-d779-4610-a7ba-2e127676db4d" +
			"&target=%2Fconcepts%2F34a571fb-d779-4610-a7ba-2e127676db4d",
		"varnish-purger/purge?target=%2Fthings%2F63ffa4d3-d7cc-4939-9bec-9ed46a78389e&target=%2Fconcepts" +
			"%2F63ffa4d3-d7cc-4939-9bec-9ed46a78389e&target=%2Fpeople%2F63ffa4d3-d7cc-4939-9bec-9ed46a78389e",
		"concept-rw-elasticsearch/memberships/ddacda04-b7cd-4d2e-86b1-7dfef0ff56a2",
	}, mockWriter.called)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(eventQueue.eventList))
}

func TestAggregateService_ProcessMessage_IndustryClassificationNotSentToEs(t *testing.T) {
	svc, _, _, eventQueue, _, _, _ := setupTestService(200, payload)
	err := svc.ProcessMessage(context.Background(), "acb19f07-bfd0-4301-a96f-ab5e5c20e533", "")
	mockWriter := svc.httpClient.(*mockHTTPClient)
	assert.Equal(t, []string{
		"concepts-rw-neo4j/industry-classifications/acb19f07-bfd0-4301-a96f-ab5e5c20e533",
		"varnish-purger/purge?target=%2Fthings%2F28090964-9997-4bc2-9638-7a11135aaff9&target=%2Fconcepts" +
			"%2F28090964-9997-4bc2-9638-7a11135aaff9&target=%2Fthings%2F34a571fb-d779-4610-a7ba-2e127676db4d" +
			"&target=%2Fconcepts%2F34a571fb-d779-4610-a7ba-2e127676db4d",
	}, mockWriter.called)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(eventQueue.eventList))
}

func TestAggregateService_ProcessMessage_Success_PurgeOnBrands(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	err := svc.ProcessMessage(context.Background(), "781bb463-dc53-4d3e-9d49-c48dc4cf6d55", "")
	mockWriter := svc.httpClient.(*mockHTTPClient)
	assert.Equal(t, []string{
		"concepts-rw-neo4j/brands/781bb463-dc53-4d3e-9d49-c48dc4cf6d55",
		"varnish-purger/purge?target=%2Fthings%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fconcepts%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fthings%2F34a571fb-d779-4610-a7ba-2e127676db4d" +
			"&target=%2Fconcepts%2F34a571fb-d779-4610-a7ba-2e127676db4d" +
			"&target=%2Fbrands%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fbrands%2F34a571fb-d779-4610-a7ba-2e127676db4d",
		"concept-rw-elasticsearch/brands/781bb463-dc53-4d3e-9d49-c48dc4cf6d55",
	}, mockWriter.called)
	assert.NoError(t, err)
}

func TestAggregateService_ProcessMessage_Success_PurgeOnOrgs(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	err := svc.ProcessMessage(context.Background(), "94659314-7eb0-423a-8030-c4abf3d6458e", "")
	mockWriter := svc.httpClient.(*mockHTTPClient)
	assert.Equal(t, []string{
		"concepts-rw-neo4j/organisations/94659314-7eb0-423a-8030-c4abf3d6458e",
		"varnish-purger/purge?target=%2Fthings%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fconcepts%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fthings%2F34a571fb-d779-4610-a7ba-2e127676db4d" +
			"&target=%2Fconcepts%2F34a571fb-d779-4610-a7ba-2e127676db4d" +
			"&target=%2Forganisations%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Forganisations%2F34a571fb-d779-4610-a7ba-2e127676db4d",
		"concept-rw-elasticsearch/organisations/94659314-7eb0-423a-8030-c4abf3d6458e",
	}, mockWriter.called)
	assert.NoError(t, err)
}

func TestAggregateService_ProcessMessage_Success_PurgeOnPublicCompany(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	err := svc.ProcessMessage(context.Background(), "e8251dab-c6d4-42d0-a4f6-430a0c565a83", "")
	mockWriter := svc.httpClient.(*mockHTTPClient)
	assert.Equal(t, []string{
		"concepts-rw-neo4j/organisations/e8251dab-c6d4-42d0-a4f6-430a0c565a83",
		"varnish-purger/purge?target=%2Fthings%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fconcepts%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Fthings%2F34a571fb-d779-4610-a7ba-2e127676db4d" +
			"&target=%2Fconcepts%2F34a571fb-d779-4610-a7ba-2e127676db4d" +
			"&target=%2Forganisations%2F28090964-9997-4bc2-9638-7a11135aaff9" +
			"&target=%2Forganisations%2F34a571fb-d779-4610-a7ba-2e127676db4d",
		"concept-rw-elasticsearch/organisations/e8251dab-c6d4-42d0-a4f6-430a0c565a83",
	}, mockWriter.called)
	assert.NoError(t, err)
}

func TestAggregateService_ProcessMessage_Success_PurgeOnMembership(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, membershipPayload)
	err := svc.ProcessMessage(context.Background(), "ce922022-8114-11e8-8f42-da24cd01f044", "")
	mockWriter := svc.httpClient.(*mockHTTPClient)
	assert.Equal(t, []string{
		"concepts-rw-neo4j/memberships/ce922022-8114-11e8-8f42-da24cd01f044",
		"varnish-purger/purge?target=%2Fthings%2Fce922022-8114-11e8-8f42-da24cd01f044" +
			"&target=%2Fconcepts%2Fce922022-8114-11e8-8f42-da24cd01f044",
		"varnish-purger/purge?target=%2Fthings%2F3b961db6-02c1-4fde-b96d-aefd339a02a6" +
			"&target=%2Fconcepts%2F3b961db6-02c1-4fde-b96d-aefd339a02a6" +
			"&target=%2Fpeople%2F3b961db6-02c1-4fde-b96d-aefd339a02a6",
	}, mockWriter.called)
	assert.NoError(t, err)
}

func TestAggregateService_ProcessMessage_GenericS3Error(t *testing.T) {
	svc, mockS3Client, _, _, _, _, _ := setupTestService(200, payload)
	mockS3Client.err = errors.New("error retrieving concept from S3")
	err := svc.ProcessMessage(context.Background(), "28090964-9997-4bc2-9638-7a11135aaff9", "")
	assert.EqualError(t, err, "error retrieving concept from S3")
}

func TestAggregateService_ProcessMessage_GenericWriterError(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(503, payload)

	err := svc.ProcessMessage(context.Background(), "28090964-9997-4bc2-9638-7a11135aaff9", "")
	assert.Error(t, err)
	assert.Equal(t, "Request to concepts-rw-neo4j/test-concepts/28090964-9997-4bc2-9638-7a11135aaff9 returned status: 503; skipping 28090964-9997-4bc2-9638-7a11135aaff9", err.Error())
}

func TestAggregateService_ProcessMessage_GenericSqsError(t *testing.T) {
	svc, _, _, mockEventQueue, _, _, _ := setupTestService(200, payload)
	mockEventQueue.err = errors.New("could not connect to SQS")

	err := svc.ProcessMessage(context.Background(), "28090964-9997-4bc2-9638-7a11135aaff9", "")
	assert.Error(t, err)
	assert.Equal(t, "could not connect to SQS", err.Error())
}

func TestAggregateService_ProcessMessage_GenericKinesisError(t *testing.T) {
	svc, _, _, _, mockKinesisClient, _, _ := setupTestService(200, payload)
	mockKinesisClient.err = errors.New("failed to add record to stream")

	err := svc.ProcessMessage(context.Background(), "28090964-9997-4bc2-9638-7a11135aaff9", "")
	assert.Error(t, err)
	assert.Equal(t, "failed to add record to stream", err.Error())
}

func TestAggregateService_ProcessMessage_S3SourceNotFoundStillWrittenAsThing(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	testUUID := "c9d3a92a-da84-11e7-a121-0401beb96201"
	err := svc.ProcessMessage(context.Background(), testUUID, "")
	assert.NoError(t, err)
	mockWriter := svc.httpClient.(*mockHTTPClient)
	actual := transform.OldAggregatedConcept{}
	err = json.NewDecoder(mockWriter.capturedBody).Decode(&actual)
	assert.NoError(t, err)
	expectedConcordedConcept := transform.OldAggregatedConcept{
		PrefUUID:  testUUID,
		PrefLabel: "TME Concept",
		Type:      "Person",
		Aliases:   []string{"TME Concept"},
		SourceRepresentations: []transform.OldConcept{
			{
				UUID:      "3a3da730-0f4c-4a20-85a6-3ebd5776bd49",
				Type:      "Thing",
				Authority: "DBPedia",
			},
			{
				UUID:           testUUID,
				Type:           "Person",
				PrefLabel:      "TME Concept",
				Authority:      "TME",
				AuthorityValue: "TME-a2f",
			},
		},
	}

	if !cmp.Equal(expectedConcordedConcept, actual) {
		diff := cmp.Diff(expectedConcordedConcept, actual)
		t.Fatal(diff)
	}
}

func TestAggregateService_ProcessMessage_S3CanonicalNotFound(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	err := svc.ProcessMessage(context.Background(), "45f278ef-91b2-45f7-9545-fbc79c1b4004", "")
	assert.EqualError(t, err, "canonical concept 45f278ef-91b2-45f7-9545-fbc79c1b4004 not found in S3")
}

func TestAggregateService_ProcessMessage_CancelContext(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := svc.ProcessMessage(ctx, "45f278ef-91b2-45f7-9545-fbc79c1b4004", "")
	assert.EqualError(t, err, "context canceled")
}

func TestAggregateService_ProcessMessage_WriterReturnsNoUuids(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, emptyPayload)

	err := svc.ProcessMessage(context.Background(), "28090964-9997-4bc2-9638-7a11135aaff9", "")
	assert.NoError(t, err)
}

func TestAggregateService_Healthchecks(t *testing.T) {
	svc, _, _, _, _, _, _ := setupTestService(200, payload)
	healthchecks := svc.Healthchecks()

	for _, v := range healthchecks {
		s, e := v.Checker()
		assert.NoError(t, e)
		assert.Equal(t, "", s)
	}
}

func TestResolveConceptType(t *testing.T) {
	person := resolveConceptType("Person")
	assert.Equal(t, "people", person)

	specialReport := resolveConceptType("SpecialReport")
	assert.Equal(t, "special-reports", specialReport)

	financialInstrument := resolveConceptType("FinancialInstrument")
	assert.Equal(t, "financial-instruments", financialInstrument)

	alphavilleSeries := resolveConceptType("AlphavilleSeries")
	assert.Equal(t, "alphaville-series", alphavilleSeries)

	topic := resolveConceptType("Topic")
	assert.Equal(t, "topics", topic)

	brand := resolveConceptType("Brand")
	assert.Equal(t, "brands", brand)

	orgs := resolveConceptType("Organisation")
	assert.Equal(t, "organisations", orgs)

	company := resolveConceptType("PublicCompany")
	assert.Equal(t, "organisations", company)
}

// getConceptFromService is a wrapper function for getting concepts from the service
//
// nolint: revive, unparam
// silence specific linters for this function as we don't care about them.
// context-as-argument: context.Context should be the first parameter of a function (revive)
// `getConceptFromService` - `bookmark` always receives `""` (unparam)
func getConceptFromService(svc *AggregateService, ctx context.Context, conceptUUID string, bookmark string) (transform.OldAggregatedConcept, string, error) {
	c, tid, err := svc.GetConcordedConcept(ctx, conceptUUID, bookmark)
	if err != nil {
		return transform.OldAggregatedConcept{}, "", err
	}
	old, err := transform.ToOldAggregateConcept(c)
	if err != nil {
		return transform.OldAggregatedConcept{}, "", err
	}
	sort.Strings(old.Aliases)
	sort.Strings(old.FormerNames)
	return old, tid, nil
}

func setupTestService(clientStatusCode int, writerResponse string) (*AggregateService, *mockS3Client, *mockSQSClient, *mockSNSClient, *mockKinesisStreamClient, chan bool, chan struct{}) {
	return setupTestServiceWithTimeout(clientStatusCode, writerResponse, time.Second*1)
}

func setupTestServiceWithTimeout(clientStatusCode int, writerResponse string, timeout time.Duration) (*AggregateService, *mockS3Client, *mockSQSClient, *mockSNSClient, *mockKinesisStreamClient, chan bool, chan struct{}) {
	s3mock := &mockS3Client{
		concepts: map[string]struct {
			transactionID string
			concept       transform.OldConcept
		}{
			"c28fa0b4-4245-11e8-842f-0ed5f89f718b": {
				transactionID: "tid_631",
				concept: transform.OldConcept{
					UUID:           "c28fa0b4-4245-11e8-842f-0ed5f89f718b",
					Type:           "PublicCompany",
					Authority:      "FACTSET",
					AuthorityValue: "B000BB-S",
					ProperName:     "Strix Group Plc",
					PrefLabel:      "Strix Group Plc",
					ShortName:      "Strix Group",
					TradeNames:     []string{"Strixy"},
					FormerNames: []string{
						"Castletown Thermostats",
						"Steam Plc",
					},
					Aliases: []string{
						"Strix Group Plc",
						"STRIX GROUP PLC",
						"Strix Group",
						"Castletown Thermostats",
						"Steam Plc",
					},
					CountryCode:                "GB",
					CountryOfRisk:              "GB",
					CountryOfIncorporation:     "IM",
					CountryOfOperations:        "GB",
					CountryOfRiskUUID:          "GB_UUID",
					CountryOfIncorporationUUID: "IM_UUID",
					CountryOfOperationsUUID:    "GB_UUID",
					PostalCode:                 "IM9 2RG",
					YearFounded:                1951,
					EmailAddress:               "info@strix.com",
					LeiCode:                    "213800KZEW5W6BZMNT62",
					ParentOrganisation:         "123",
				},
			},
			"99247059-04ec-3abb-8693-a0b8951fdcab": {
				transactionID: "tid_123",
				concept: transform.OldConcept{
					UUID:           "99247059-04eFc-3abb-8693-a0b8951fdcab",
					PrefLabel:      "Test Concept",
					Authority:      "Smartlogic",
					AuthorityValue: "99247059-04ec-3abb-8693-a0b8951fdcab",
					Type:           "Person",
					Salutation:     "Mr",
					BirthYear:      2018,
				},
			},
			"28090964-9997-4bc2-9638-7a11135aaff9": {
				transactionID: "tid_456",
				concept: transform.OldConcept{
					UUID:           "28090964-9997-4bc2-9638-7a11135aaff9",
					PrefLabel:      "Root Concept",
					Authority:      "Smartlogic",
					AuthorityValue: "28090964-9997-4bc2-9638-7a11135aaff9",
					Type:           "TestConcept",
					FacebookPage:   "facebook/smartlogicPerson",
					TwitterHandle:  "@FtSmartlogicPerson",
					ScopeNote:      "This note is in scope",
					EmailAddress:   "person123@ft.com",
					ShortLabel:     "Concept",
					MembershipRoles: []transform.MembershipRole{
						{
							RoleUUID:        "ccdff192-4d6c-4539-bbe8-7e24e81ed49e",
							InceptionDate:   "2002-06-01",
							TerminationDate: "2011-11-29",
						},
					},
					OrganisationUUID: "a4528fc9-0615-4bfa-bc99-596ea1ddec28",
					PersonUUID:       "973509c1-5238-4c83-9a7d-89009e839ff8",
					InceptionDate:    "2002-06-01",
					TerminationDate:  "2011-11-29",
					FigiCode:         "BBG000Y1HJT8",
					IssuedBy:         "613b1f72-cc74-4d8f-9406-28fc91b82a2a",
				},
			},
			"34a571fb-d779-4610-a7ba-2e127676db4d": {
				transactionID: "tid_789",
				concept: transform.OldConcept{
					UUID:           "34a571fb-d779-4610-a7ba-2e127676db4d",
					PrefLabel:      "TME Concept",
					Authority:      "TME",
					AuthorityValue: "TME-123",
					Type:           "TestConcept",
					IsDeprecated:   true,
				},
			},
			"28090964-9997-4bc2-9638-7a11135aaf10": {
				transactionID: "tid_456",
				concept: transform.OldConcept{
					UUID:           "28090964-9997-4bc2-9638-7a11135aaf10",
					PrefLabel:      "Root Concept",
					Authority:      "Smartlogic",
					AuthorityValue: "28090964-9997-4bc2-9638-7a11135aaf10",
					Type:           "TestConcept",
					FacebookPage:   "facebook/smartlogicPerson",
					TwitterHandle:  "@FtSmartlogicPerson",
					ScopeNote:      "This note is in scope",
					EmailAddress:   "person123@ft.com",
					ShortLabel:     "Concept",
					MembershipRoles: []transform.MembershipRole{
						{
							RoleUUID:        "ccdff192-4d6c-4539-bbe8-7e24e81ed49e",
							InceptionDate:   "2002-06-01",
							TerminationDate: "2011-11-29",
						},
					},
					OrganisationUUID: "a4528fc9-0615-4bfa-bc99-596ea1ddec28",
					PersonUUID:       "973509c1-5238-4c83-9a7d-89009e839ff8",
					InceptionDate:    "2002-06-01",
					TerminationDate:  "2011-11-29",
					FigiCode:         "BBG000Y1HJT8",
					IssuedBy:         "613b1f72-cc74-4d8f-9406-28fc91b82a2a",
					IsDeprecated:     true,
				},
			},
			"34a571fb-d779-4610-a7ba-2e127676db4e": {
				transactionID: "tid_789",
				concept: transform.OldConcept{
					UUID:           "34a571fb-d779-4610-a7ba-2e127676db4e",
					PrefLabel:      "TME Concept",
					Authority:      "TME",
					AuthorityValue: "TME-123",
					Type:           "TestConcept",
					IsDeprecated:   false,
				},
			},
			"28090964-9997-4bc2-9638-7a11135aaf11": {
				transactionID: "tid_456",
				concept: transform.OldConcept{
					UUID:           "28090964-9997-4bc2-9638-7a11135aaf11",
					PrefLabel:      "Root Concept",
					Authority:      "Smartlogic",
					AuthorityValue: "28090964-9997-4bc2-9638-7a11135aaf11",
					Type:           "TestConcept",
					FacebookPage:   "facebook/smartlogicPerson",
					TwitterHandle:  "@FtSmartlogicPerson",
					ScopeNote:      "This note is in scope",
					EmailAddress:   "person123@ft.com",
					ShortLabel:     "Concept",
					SupersededByUUIDs: []string{
						"28090964-9997-4bc2-9638-7a11135aaff9",
					},
					MembershipRoles: []transform.MembershipRole{
						{
							RoleUUID:        "ccdff192-4d6c-4539-bbe8-7e24e81ed49e",
							InceptionDate:   "2002-06-01",
							TerminationDate: "2011-11-29",
						},
					},
					OrganisationUUID: "a4528fc9-0615-4bfa-bc99-596ea1ddec28",
					PersonUUID:       "973509c1-5238-4c83-9a7d-89009e839ff8",
					InceptionDate:    "2002-06-01",
					TerminationDate:  "2011-11-29",
					FigiCode:         "BBG000Y1HJT8",
					IssuedBy:         "613b1f72-cc74-4d8f-9406-28fc91b82a2a",
					IsDeprecated:     true,
				},
			},
			"c9d3a92a-da84-11e7-a121-0401beb96201": {
				transactionID: "tid_629",
				concept: transform.OldConcept{
					UUID:           "c9d3a92a-da84-11e7-a121-0401beb96201",
					PrefLabel:      "TME Concept",
					Authority:      "TME",
					AuthorityValue: "TME-a2f",
					Type:           "Person",
				},
			},
			"99309d51-8969-4a1e-8346-d51f1981479b": {
				transactionID: "tid_439",
				concept: transform.OldConcept{
					UUID:           "99309d51-8969-4a1e-8346-d51f1981479b",
					PrefLabel:      "Lone TME Concept",
					Authority:      "TME",
					AuthorityValue: "TME-qwe",
					Type:           "Person",
				},
			},
			"6562674e-dbfa-4cb0-85b2-41b0948b7cc2": {
				transactionID: "tid_630",
				concept: transform.OldConcept{
					UUID:           "6562674e-dbfa-4cb0-85b2-41b0948b7cc2",
					PrefLabel:      "Some random financial instrument",
					Authority:      "FACTSET",
					AuthorityValue: "B000BB-S",
					Type:           "FinancialInstrument",
					FigiCode:       "BBG000Y1HJT8",
					IssuedBy:       "4e484678-cf47-4168-b844-6adb47f8eb58",
				},
			},
			"344fdb1d-0585-31f7-814f-b478e54dbe1f": {
				transactionID: "tid_631",
				concept: transform.OldConcept{
					UUID:           "344fdb1d-0585-31f7-814f-b478e54dbe1f",
					PrefLabel:      "Director/Board Member",
					Authority:      "FACTSET",
					AuthorityValue: "BRD",
					Type:           "BoardRole",
				},
			},
			"87cda39a-e354-3dfb-b28a-b9a04887577b": {
				transactionID: "tid_632",
				concept: transform.OldConcept{
					UUID:             "87cda39a-e354-3dfb-b28a-b9a04887577b",
					PrefLabel:        "Independent Non-Executive Director",
					Authority:        "FACTSET",
					AuthorityValue:   "1000016",
					Type:             "Membership",
					PersonUUID:       "d4050b35-45ac-3933-9fad-7720a0dce8df",
					OrganisationUUID: "064ce159-8835-3426-b456-c86d48de8511",
					InceptionDate:    "2002-06-01",
					TerminationDate:  "2011-11-30",
					MembershipRoles: []transform.MembershipRole{
						{

							RoleUUID:        "344fdb1d-0585-31f7-814f-b478e54dbe1f",
							InceptionDate:   "2002-06-01",
							TerminationDate: "2011-11-29",
						},
						{
							RoleUUID:        "abacb0e1-3f7e-334a-96b9-ed5da35f3251",
							InceptionDate:   "2011-07-26",
							TerminationDate: "2011-11-29",
						},
					},
				},
			},
			"781bb463-dc53-4d3e-9d49-c48dc4cf6d55": {
				transactionID: "tid_633",
				concept: transform.OldConcept{
					UUID:           "781bb463-dc53-4d3e-9d49-c48dc4cf6d55",
					PrefLabel:      "Test FT Brand",
					Authority:      "Smartlogic",
					AuthorityValue: "781bb463-dc53-4d3e-9d49-c48dc4cf6d55",
					Type:           "Brand",
					DescriptionXML: "<body>The best brand</body>",
					Strapline:      "The Best Brand",
					ImageURL:       "localhost:8080/12345",
					ParentUUIDs:    []string{"ec467314-63cf-4976-a124-77175d10423d"},
					BroaderUUIDs:   []string{"575a2223-6307-4000-8882-935c27f4e8bb"},
					RelatedUUIDs:   []string{"b73e632c-9b8d-477d-bb45-aaf574bc015c"},
					ImpliedByUUIDs: []string{"b5d7c6b5-db7d-4bce-9d6a-f62195571f92"},
					HasFocusUUIDs:  []string{"2e7429bd-7a84-41cb-a619-2c702893e359"},
				},
			},
			"94659314-7eb0-423a-8030-c4abf3d6458e": {
				transactionID: "tid_634",
				concept: transform.OldConcept{
					UUID:           "94659314-7eb0-423a-8030-c4abf3d6458e",
					PrefLabel:      "Test FT Organisation",
					Authority:      "Smartlogic",
					AuthorityValue: "94659314-7eb0-423a-8030-c4abf3d6458e5",
					Type:           "Organisation",
				},
			},
			"e8251dab-c6d4-42d0-a4f6-430a0c565a83": {
				transactionID: "tid_635",
				concept: transform.OldConcept{
					UUID:           "e8251dab-c6d4-42d0-a4f6-430a0c565a83",
					PrefLabel:      "Test FT Public Company",
					Authority:      "Smartlogic",
					AuthorityValue: "e8251dab-c6d4-42d0-a4f6-430a0c565a83",
					Type:           "PublicCompany",
				},
			},
			"a141f50f-31d7-4f89-8143-eec971e54ba8": {
				transactionID: "tid_636",
				concept: transform.OldConcept{
					UUID:           "a141f50f-31d7-4f89-8143-eec971e54ba8",
					PrefLabel:      "Test FT Concorded Organisation",
					Authority:      "Smartlogic",
					AuthorityValue: "a141f50f-31d7-4f89-8143-eec971e54ba8",
					Type:           "Organisation",
				},
			},
			"ce922022-8114-11e8-8f42-da24cd01f044": {
				transactionID: "tid_637",
				concept: transform.OldConcept{
					UUID:             "ce922022-8114-11e8-8f42-da24cd01f044",
					PrefLabel:        "Test Membership",
					Authority:        "FACTSET",
					AuthorityValue:   "100001-E",
					Type:             "Membership",
					PersonUUID:       "3b961db6-02c1-4fde-b96d-aefd339a02a6",
					OrganisationUUID: "064ce159-8835-3426-b456-c86d48de8511",
					InceptionDate:    inceptionDate,
					TerminationDate:  "2009-12-31",
					MembershipRoles: []transform.MembershipRole{
						{

							RoleUUID:        "344fdb1d-0585-31f7-814f-b478e54dbe1f",
							InceptionDate:   inceptionDate,
							TerminationDate: "2009-12-31",
						},
					},
				},
			},
			"01e284c2-7d77-4df6-8df7-57ec006194a4": {
				transactionID: "tid_854",
				concept: transform.OldConcept{
					UUID:           "01e284c2-7d77-4df6-8df7-57ec006194a4",
					PrefLabel:      "Czar of the Universe",
					Authority:      "FACTSET",
					AuthorityValue: "CZR",
					Type:           "MembershipRole",
				},
			},
			"f784be91-601a-42db-ac57-e1d5da8b4866": {
				transactionID: "tid_824",
				concept: transform.OldConcept{
					UUID:             "f784be91-601a-42db-ac57-e1d5da8b4866",
					PrefLabel:        "Supreme Ruler",
					Authority:        "FACTSET",
					AuthorityValue:   "46987235",
					Type:             "Membership",
					OrganisationUUID: "a141f50f-31d7-4f89-8143-eec971e54ba8",
					PersonUUID:       "99309d51-8969-4a1e-8346-d51f1981479b",
					MembershipRoles: []transform.MembershipRole{
						{
							RoleUUID: "01e284c2-7d77-4df6-8df7-57ec006194a4",
						},
					},
				},
			},
			"ddacda04-b7cd-4d2e-86b1-7dfef0ff56a2": {
				transactionID: "tid_771",
				concept: transform.OldConcept{
					UUID:             "ddacda04-b7cd-4d2e-86b1-7dfef0ff56a2",
					PrefLabel:        "Author McAuthorface",
					Authority:        "Smartlogic",
					AuthorityValue:   "ddacda04-b7cd-4d2e-86b1-7dfef0ff56a2",
					Type:             "Membership",
					OrganisationUUID: "9d4be817-dab9-4292-acf8-32416ebe9e94",
					PersonUUID:       "63ffa4d3-d7cc-4939-9bec-9ed46a78389e",
					MembershipRoles: []transform.MembershipRole{
						{
							RoleUUID: "8e8a8be0-be14-4c57-860e-f3ea35d68249",
						},
					},
				},
			},
			"f8024a12-2d71-4f0e-996d-bcbc07df3921": {
				transactionID: "tid_999",
				concept: transform.OldConcept{
					UUID:           "f8024a12-2d71-4f0e-996d-bcbc07df3921",
					PrefLabel:      "Paris",
					Authority:      "Smartlogic",
					AuthorityValue: "f8024a12-2d71-4f0e-996d-bcbc07df3921",
					Type:           "Location",
				},
			},
			"900dd202-fccc-3280-b053-d46c234dcbe2": {
				transactionID: "tid_999_1",
				concept: transform.OldConcept{
					UUID:           "900dd202-fccc-3280-b053-d46c234dcbe2",
					PrefLabel:      "Paris, Texas",
					Authority:      "TME",
					AuthorityValue: "UGFyaXMsIFRleGFz-R0w=",
					Type:           "Location",
				},
			},
			"c78371f2-1f55-4099-ae63-f44e44bb2af8": { // FR_ML_UUID
				transactionID: "tid_112",
				concept: transform.OldConcept{
					UUID:           "c78371f2-1f55-4099-ae63-f44e44bb2af8",
					PrefLabel:      "France",
					Authority:      "ManagedLocation",
					AuthorityValue: "c78371f2-1f55-4099-ae63-f44e44bb2af8",
					Type:           "Location",
					ISO31661:       "FR",
				},
			},
			"FR_TME_UUID": {
				transactionID: "tid_112_1",
				concept: transform.OldConcept{
					UUID:           "FR_TME_UUID",
					PrefLabel:      "French Republic",
					Authority:      "TME",
					AuthorityValue: "FR_TME_AUTH_VALUE",
					Type:           "Location",
				},
			},
			"cc3bf637-9288-499b-9221-bb6e8e003f03": { // BE_SL_UUID
				transactionID: "tid_358",
				concept: transform.OldConcept{
					UUID:           "cc3bf637-9288-499b-9221-bb6e8e003f03",
					PrefLabel:      "Belgium",
					Authority:      "Smartlogic",
					AuthorityValue: "cc3bf637-9288-499b-9221-bb6e8e003f03",
					Type:           "Location",
				},
			},
			"BE_ML_UUID": {
				transactionID: "tid_358_1",
				concept: transform.OldConcept{
					UUID:           "BE_ML_UUID",
					PrefLabel:      "Kingdom of Belgium",
					Authority:      "ManagedLocation",
					AuthorityValue: "BE_ML_UUID",
					Type:           "Location",
					ISO31661:       "BE",
				},
			},
			"BE_TME_UUID": {
				transactionID: "tid_358_2",
				concept: transform.OldConcept{
					UUID:           "BE_TME_UUID",
					PrefLabel:      "Royaume de Belgique",
					Authority:      "TME",
					AuthorityValue: "BE_TME_AUTH_VALUE",
					Type:           "Location",
				},
			},
			"acb19f07-bfd0-4301-a96f-ab5e5c20e533": {
				transactionID: "tid_359",
				concept: transform.OldConcept{
					UUID:               "acb19f07-bfd0-4301-a96f-ab5e5c20e533",
					PrefLabel:          "Newspaper, Periodical, Book, and Directory Publishers",
					IndustryIdentifier: "5111",
					Authority:          "Smartlogic",
					AuthorityValue:     "acb19f07-bfd0-4301-a96f-ab5e5c20e533",
					Type:               "NAICSIndustryClassification",
				},
			},
			"Organisation_WithNAICSCodes_Factset_UUID": {
				transactionID: "tid_735",
				concept: transform.OldConcept{
					UUID:           "Organisation_WithNAICSCodes_Factset_UUID",
					Type:           "PublicCompany",
					Authority:      "FACTSET",
					AuthorityValue: "000C7F-E",
					PrefLabel:      "Apple, Inc.",
					NAICSIndustryClassifications: []transform.NAICSIndustryClassification{
						{
							UUID: "25c3be2a-15e0-434e-aaa9-ca067e70ae11",
							Rank: 1,
						},
						{
							UUID: "c9cae3ee-a804-4001-8046-02e714e1fa5b",
							Rank: 2,
						},
						{
							UUID: "7f0134c1-606a-4d12-a455-661cc4b0bfac",
							Rank: 3,
						},
						{
							UUID: "c35b851c-d185-40bd-967c-2403084920b3",
							Rank: 4,
						},
					},
				},
			},
			"1b72a837-b6a6-4490-85e2-c174101bfa10": { //Organisation_WithNAICSCodes_Smartlogic_UUID
				transactionID: "tid_736",
				concept: transform.OldConcept{
					UUID:           "1b72a837-b6a6-4490-85e2-c174101bfa10",
					Type:           "PublicCompany",
					Authority:      "Smartlogic",
					AuthorityValue: "000C7F-E",
					PrefLabel:      "Apple, Inc.",
				},
			},
		},
	}

	externalS3Mock := &mockS3Client{
		concepts: map[string]struct {
			transactionID string
			concept       transform.OldConcept
		}{
			"929da855-c1ba-4576-89c1-5c3ec9e4c6ef/f3633e04-2ee3-48ce-8081-37734dab3fdc": {
				transactionID: "tid_358",
				concept: transform.OldConcept{
					UUID:           "f3633e04-2ee3-48ce-8081-37734dab3fdc",
					PrefLabel:      "Capital Flows",
					Authority:      "929da855-c1ba-4576-89c1-5c3ec9e4c6ef",
					AuthorityValue: "f3633e04-2ee3-48ce-8081-37734dab3fdc",
					Type:           "TestConcept",
				},
			},
		},
	}

	conceptsQueue := &mockSQSClient{
		conceptsQueue: map[string]string{
			"1": "99247059-04ec-3abb-8693-a0b8951fdcab",
		},
	}
	eventsSNS := &mockSNSClient{}
	concordClient := &mockConcordancesClient{
		concordances: map[string][]concordances.ConcordanceRecord{
			"f8024a12-2d71-4f0e-996d-bcbc07df3921": {
				{
					UUID:      "f8024a12-2d71-4f0e-996d-bcbc07df3921",
					Authority: "Smartlogic",
				},
				{
					UUID:      "900dd202-fccc-3280-b053-d46c234dcbe2",
					Authority: "TME",
				},
			},
			"c78371f2-1f55-4099-ae63-f44e44bb2af8": {
				{
					UUID:      "c78371f2-1f55-4099-ae63-f44e44bb2af8",
					Authority: "ManagedLocation",
				},
				{
					UUID:      "FR_TME_UUID",
					Authority: "TME",
				},
			},
			"cc3bf637-9288-499b-9221-bb6e8e003f03": { // BE_SL_UUID
				{
					UUID:      "cc3bf637-9288-499b-9221-bb6e8e003f03",
					Authority: "Smartlogic",
				},
				{
					UUID:      "BE_ML_UUID",
					Authority: "ManagedLocation",
				},
				{
					UUID:      "BE_TME_UUID",
					Authority: "TME",
				},
			},
			"28090964-9997-4bc2-9638-7a11135aaff9": []concordances.ConcordanceRecord{
				concordances.ConcordanceRecord{
					UUID:      "28090964-9997-4bc2-9638-7a11135aaff9",
					Authority: "Smartlogic",
				},
				concordances.ConcordanceRecord{
					UUID:      "34a571fb-d779-4610-a7ba-2e127676db4d",
					Authority: "FT-TME",
				},
			},
			"28090964-9997-4bc2-9638-7a11135aaf10": []concordances.ConcordanceRecord{
				concordances.ConcordanceRecord{
					UUID:      "28090964-9997-4bc2-9638-7a11135aaf10",
					Authority: "Smartlogic",
				},
				concordances.ConcordanceRecord{
					UUID:      "34a571fb-d779-4610-a7ba-2e127676db4e",
					Authority: "FT-TME",
				},
			},
			"c9d3a92a-da84-11e7-a121-0401beb96201": []concordances.ConcordanceRecord{
				concordances.ConcordanceRecord{
					UUID:      "c9d3a92a-da84-11e7-a121-0401beb96201",
					Authority: "Smartlogic",
				},
				concordances.ConcordanceRecord{
					UUID:      "3a3da730-0f4c-4a20-85a6-3ebd5776bd49",
					Authority: "DBPedia",
				},
			},
			"4a4aaca0-b059-426c-bf4f-f00c6ef940ae": []concordances.ConcordanceRecord{
				concordances.ConcordanceRecord{
					UUID:      "4a4aaca0-b059-426c-bf4f-f00c6ef940ae",
					Authority: "Smartlogic",
				},
				concordances.ConcordanceRecord{
					UUID:      "3a3da730-0f4c-4a20-85a6-3ebd5776bd49",
					Authority: "FT-TME",
				},
			},
			"a141f50f-31d7-4f89-8143-eec971e54ba8": []concordances.ConcordanceRecord{
				concordances.ConcordanceRecord{
					UUID:      "a141f50f-31d7-4f89-8143-eec971e54ba8",
					Authority: "Smartlogic",
				},
				concordances.ConcordanceRecord{
					UUID:      "c28fa0b4-4245-11e8-842f-0ed5f89f718b",
					Authority: "FACTSET",
				},
			},
			"99309d51-8969-4a1e-8346-d51f1981479b": []concordances.ConcordanceRecord{
				concordances.ConcordanceRecord{
					UUID:      "99309d51-8969-4a1e-8346-d51f1981479b",
					Authority: "TME",
				},
			},
			"1b72a837-b6a6-4490-85e2-c174101bfa10": { // Organisation_WithNAICSCodes_Smartlogic_UUID
				{
					UUID:      "1b72a837-b6a6-4490-85e2-c174101bfa10",
					Authority: "Smartlogic",
				},
				{
					UUID:      "Organisation_WithNAICSCodes_Factset_UUID",
					Authority: "FACTSET",
				},
			},
		},
	}

	kinesis := &mockKinesisStreamClient{}
	feedback := make(chan bool)
	done := make(chan struct{})

	svc := NewService(s3mock, externalS3Mock, conceptsQueue, eventsSNS, concordClient, kinesis,
		neo4jUrl,
		esUrl,
		varnishPurgerUrl,
		[]string{"Person", "Brand", "PublicCompany", "Organisation"},
		&mockHTTPClient{
			resp:       writerResponse,
			statusCode: clientStatusCode,
			err:        nil,
			called:     []string{},
		},
		feedback,
		done,
		timeout,
		false,
	)

	feedback <- true
	for len(feedback) > 0 {
		time.Sleep(100 * time.Nanosecond)
	}
	return svc, s3mock, conceptsQueue, eventsSNS, kinesis, feedback, done
}
