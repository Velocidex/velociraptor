import "./search.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import FormGroup from 'react-bootstrap/FormGroup';
import { withRouter }  from "react-router-dom";
import Autosuggest from 'react-autosuggest';

import api from '../core/api-service.js';

class VeloClientSearch extends Component {
    static propTypes = {
        // Update the applications's search parameter.
        setSearch: PropTypes.func.isRequired,
    };

    componentDidMount = () => {
        let query = this.props.match && this.props.match.params &&
            this.props.match.params.query;
        if (query && query !== this.state.query) {
            this.this.setState({query: query});
        };
    };

    state = {
        // query used to update suggestions.
        query: "",
        options: [],
    }

    showAll = () => {
        this.setState({query: "all"});
        this.props.setSearch("all");
        this.props.history.push('/search/all');
    }

    setQuery = (query) => {
        this.setState({query: query});
        this.props.setSearch(query);
        this.props.history.push('/search/' + (query || "all"));
    }

    showSuggestions = (query) => {
        api.get('v1/SearchClients', {
            query: query + "*",
            count: 10,
            type:1,

        }).then(resp => {
            if (resp.data && resp.data.names) {
                this.setState({
                    options: resp.data.names,
                });
            }
            return true;
        });
    }

    render() {
        return (
              <FormGroup>
                <ButtonGroup>
                  <Autosuggest
                    suggestions={this.state.options}
                    onSuggestionsFetchRequested={(x) => this.showSuggestions(x.value)}
                    onSuggestionsClearRequested={() => this.setState({options: []})}
                    onSuggestionSelected={(e, x) => this.setQuery(x.suggestionValue)}
                    getSuggestionValue={x=>x}
                    renderSuggestion={(x) => <div className="search-suggestions">{x}</div>}
                    inputProps={{
                        placeholder: "Search clients",
                        value: this.state.query,
                        onChange: (e) => {
                            this.setState({query: e.currentTarget.value});
                        },
                    }}

                  />
                  <Button id="client_query_submit"
                          onClick={() => this.setQuery(this.state.query)}
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
        );
    }
};


export default withRouter(VeloClientSearch);
