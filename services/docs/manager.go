package docs

import (
	"context"
	"sync"

	"github.com/Velocidex/velociraptor-site-search/api"
	"github.com/blevesearch/bleve"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

type DocManager struct {
	mu sync.Mutex

	config_obj *config_proto.Config

	index api.Index
}

func (self *DocManager) Search(
	ctx context.Context,
	query_str string, start, len int) (*api_proto.DocSearchResponses, error) {

	index, err := self.GetIndex(ctx)
	if err != nil {
		return nil, err
	}

	query := bleve.NewQueryStringQuery(query_str)
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Fields = []string{"title", "text",
		"url", "tags", "rank", "crumbs"}
	searchRequest.Highlight = bleve.NewHighlight()
	searchRequest.From = start
	searchRequest.Size = len
	searchRequest.SortBy([]string{"-rank", "-_score"})

	searchResult, err := index.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	res := &api_proto.DocSearchResponses{
		Total: searchResult.Total,
	}

	for _, i := range searchResult.Hits {
		page, err := api.PageFromFields(i.Fields)
		if err != nil {
			continue
		}

		item := &api_proto.DocSearchResponse{
			Title:    page.Title,
			FullText: page.Text,
			Link:     page.Url,
			Tags:     page.Tags,
			Crumbs:   page.BreadCrumbs,
		}

		// We only highlight the Text field
		locations, pres := i.Locations["text"]
		if pres {
			for _, hit_locations := range locations {
				for _, loc := range hit_locations {
					item.Highlights = append(item.Highlights, &api_proto.Highlight{
						Start: loc.Start,
						End:   loc.End,
					})
				}
			}
		}

		res.Items = append(res.Items, item)
	}

	return res, nil
}

func NewDocManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.DocManager, error) {

	if utils.IsRootOrg(config_obj.OrgId) {
		return &DocManager{
			config_obj: config_obj,
		}, nil
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	root_org_config, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return nil, err
	}

	return services.GetDocManager(root_org_config)
}
