import "./search.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import FormGroup from 'react-bootstrap/FormGroup';
import Form from 'react-bootstrap/Form';
import { AsyncTypeahead } from 'react-bootstrap-typeahead';
import { withRouter }  from "react-router-dom";

import api from '../core/api-service.js';

class VeloClientSearch extends Component {
    static propTypes = {
        // Update the applications's search parameter.
        setSearch: PropTypes.func.isRequired,
    };

    state = {
        // query used to update suggestions.
        query: "",
        options: [],
    }

    showAll = () => {
        this.setState({query: "all"});
        this.props.setSearch("all");
        this.props.history.push('/search?query=all');
    }

    setQuery = (query) => {
        this.setState({query: query});
        this.props.setSearch(query);
        this.props.history.push('/search?query=' + (query || "all"));
    }

    showSuggestions = (query) => {
        this.setState({query: query});
        api.get('/api/v1/SearchClients', {
            query: query + "*",
            count: 10,
            type:1,

        }).then(resp => {
            if (resp.data && resp.data.names) {
                this.setState({
                    query: query,
                    options: resp.data.names,
                });
            }
            return true;
        });
    }

    render() {
        return (
            <Form>
              <FormGroup>
                <ButtonGroup>
                  <AsyncTypeahead
                    autoFocus={true}
                    defaultInputValue={this.state.query || ""}
                    placeholder="Search clients"
                    id="searchbox"
                    isLoading={false}
                    options={this.state.options || []}
                    onChange={(x) => {
                        if (x.length > 0) {
                            this.setQuery(x[0]);
                        }
                    }}
                    onSearch={this.showSuggestions}
                  />
                  <Button id="client_query_submit"
                          onClick={(e) => this.setQuery(e.currentTarget.value)}
                          variant="default" type="submit">
                    <FontAwesomeIcon icon="search"/>
                  </Button>
                  <Button id="show-hosts-btn"
                          onClick={(e) => this.setQuery("all")}
                          variant="default" type="button">
                    <FontAwesomeIcon icon="server"/>
                    <span className="button-label">Show All</span>
                  </Button>
                </ButtonGroup>
              </FormGroup>
            </Form>
        );
    }
};


export default withRouter(VeloClientSearch);
