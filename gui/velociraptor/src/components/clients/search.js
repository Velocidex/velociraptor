import "./search.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import FormGroup from 'react-bootstrap/FormGroup';
import { withRouter }  from "react-router-dom";
import Autosuggest from 'react-autosuggest';
import Dropdown from 'react-bootstrap/Dropdown';
import UserConfig from '../core/user.js';

import api from '../core/api-service.js';
import axios from 'axios';
import T from '../i8n/i8n.js';

class VeloClientSearch extends Component {
    static contextType = UserConfig;
    static propTypes = {
        // Update the applications's search parameter.
        setSearch: PropTypes.func.isRequired,
    };

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        let query = this.props.match && this.props.match.params &&
            this.props.match.params.query;
        if (query && query !== this.state.query) {
            this.this.setState({query: query});
        };
    };

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    state = {
        // query used to update suggestions.
        query: "",
        options: [],
    }

    showAll = () => {
        this.setState({query: "all"});
        this.props.setSearch("all");
    }

    setQuery = (query) => {
        this.setState({query: query});
        this.props.setSearch(query);
    }

    showSuggestions = (query) => {
        api.get('v1/SearchClients', {
            query: query,
            limit: 10,
            name_only: true,

        }, this.source.token).then(resp => {
            if (resp.data && resp.data.names) {
                let options = resp.data.names;
                this.setState({
                    options: options,
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
                        placeholder: T("SEARCH_CLIENTS"),
                        spellCheck: "false",
                        id: this.props.id || "client-search-bar",
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
                  <Dropdown>
                    <Dropdown.Toggle variant="default">
                    </Dropdown.Toggle>
                    <Dropdown.Menu>
                      <Dropdown.Item
                        id="show-hosts-btn"
                        onClick={(e) => this.setQuery("all")}
                        variant="default" type="button">
                        <FontAwesomeIcon icon="server"/>
                        <span className="button-label">Show All</span>
                      </Dropdown.Item>
                      <Dropdown.Item
                        title="Recent"
                        onClick={(e) => this.setQuery(
                            "recent:" + this.context.traits.username)}
                        variant="default" type="button">
                        <FontAwesomeIcon icon="bookmark"/>
                        <span className="button-label">Recent Hosts</span>
                      </Dropdown.Item>
                    </Dropdown.Menu>
                  </Dropdown>
                </ButtonGroup>
              </FormGroup>
        );
    }
};


export default withRouter(VeloClientSearch);
