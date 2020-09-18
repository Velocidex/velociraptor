import React, { Component } from 'react';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import InputGroup from 'react-bootstrap/InputGroup';
import FormGroup from 'react-bootstrap/FormGroup';
import Form from 'react-bootstrap/Form';
import { AsyncTypeahead } from 'react-bootstrap-typeahead';

import api from '../core/api-service.js';

export default class VeloClientSearch extends Component {
    state = {};

    render() {
        return (
              <Form>
                <FormGroup>
                  <ButtonGroup>
                    <AsyncTypeahead
                      id="searchbox"
                      isLoading={this.state.isLoading || false}
                      options={this.state.options || []}
                      onSearch={(query) => {
                          this.setState({isLoading: true});
                          api.get('/api/v1/SearchClients', {
                              query: query + "*",
                              limit: 10,
                              type:1,

                          }).then(resp => {
                              if (resp.data && resp.data.names) {
                                  this.setState({
                                      isLoading: false,
                                      options: resp.data.names,
                                  });
                              }
                              return true;
                          });
                      }}
                      />
                    <Button id="client_query_submit"
                            variant="default"
                            ng-click="controller.submitQuery($event)">
                      <FontAwesomeIcon icon="search"/>
                    </Button>
                    <Button id="show-hosts-btn"
                            variant="default"
                            ng-click="controller.query='all'; controller.submitQuery($event)">
                      <FontAwesomeIcon icon="server"/>
                    </Button>
                  </ButtonGroup>
                </FormGroup>
              </Form>
        );
    }
};
