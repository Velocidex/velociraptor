import "./search.css";
import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import FormGroup from 'react-bootstrap/FormGroup';
import Form from 'react-bootstrap/Form';
import Autosuggest from 'react-autosuggest';
import Dropdown from 'react-bootstrap/Dropdown';
import UserConfig from '../core/user.jsx';

import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import T from '../i8n/i8n.jsx';


export default class VeloClientSearch extends Component {
    static contextType = UserConfig;
    static propTypes = {
        // Update the applications's search parameter.
        setSearch: PropTypes.func.isRequired,
    };

    componentDidMount = () => {
        this.source = CancelToken.source();
        let query = this.getQueryFromLocation();
        if (query && query !== this.state.query) {
            this.setState({query: query});
        };
    };

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let query = this.getQueryFromLocation();
        if (query && query !== this.state.query) {
            this.setQuery(query);
        };
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    state = {
        // query used to update suggestions.
        query: "",

        // When the user begins editing the query string we set the
        // new query here. When the user submits the query, we clear
        // this and set the query in the upstream component.

        // When the edit box is editing, we block automatic updates of
        // the query content from the URL.
        pending_query: null,
        options: [],
    }

    getQueryFromLocation = ()=>{
        let hash = window.location.hash || "";
        if(hash.startsWith("#/search/")) {
            return hash.substring(9);
        };
        return "";
    }

    showAll = () => {
        this.setState({query: "all"});
        this.props.setSearch("all");
    }

    setQuery = (query) => {
        if(_.isString(this.state.pending_query)) {
            query = this.state.pending_query;
        }
        this.setState({query: query, pending_query: null});
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

    // Ensure the query string is a string.
    getQuery = ()=>{
        if(_.isString(this.state.pending_query)) {
            return this.state.pending_query;
        }
        if(_.isString(this.state.query)) {
            return this.state.query;
        }
        return "";
    };

    render() {
        return (
            <Form onSubmit={e=>{
                e.preventDefault();
                return false;
            }}>
              <FormGroup>
                <ButtonGroup>
                  <Autosuggest
                    suggestions={this.state.options}
                    onSuggestionsFetchRequested={(x) => this.showSuggestions(x.value)}
                    onSuggestionsClearRequested={() => this.setState({options: []})}
                    onSuggestionSelected={(e, x) => {
                        this.setState({pending_query: null});
                        this.props.setSearch(x.suggestionValue);
                    }}
                    getSuggestionValue={x=>x}
                    renderSuggestion={(x) => <div className="search-suggestions">{x}</div>}
                    inputProps={{
                        placeholder: T("SEARCH_CLIENTS"),
                        spellCheck: "false",
                        value: this.getQuery(),
                        id: "client-search-bar",
                        onChange: (e, {newValue, method}) => {
                            this.setState({pending_query: newValue});
                            e.preventDefault();
                            return false;
                        },
                        onBlur: ()=>this.setQuery(this.state.pending_query),
                    }}

                  />
                  <Button id="client_query_submit"
                          onClick={() => this.setQuery(this.state.query)}
                          variant="default" type="submit">
                    <FontAwesomeIcon icon="search"/>
                    <span className="sr-only">{T("Search")}</span>
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
                        <span className="button-label">{T("Show All")}</span>
                      </Dropdown.Item>
                      <Dropdown.Item
                        onClick={(e) => this.setQuery(
                            "recent:" + this.context.traits.username)}
                        variant="default" type="button">
                        <FontAwesomeIcon icon="bookmark"/>
                        <span className="button-label">{T("Recent Hosts")}</span>
                      </Dropdown.Item>
                      <Dropdown.Item
                        onClick={(e) => this.setQuery("label:*")}
                        variant="default" type="button">
                        <FontAwesomeIcon icon="tags"/>
                        <span className="button-label">{T("Labeled Hosts")}</span>
                      </Dropdown.Item>
                      <Dropdown.Item
                        onClick={(e) => this.setQuery("label:none")}
                        variant="default" type="button">
                        <FontAwesomeIcon icon="tags"/>
                        <span className="button-label">{T("Unlabeled Hosts")}</span>
                      </Dropdown.Item>
                    </Dropdown.Menu>
                  </Dropdown>
                </ButtonGroup>
              </FormGroup>
            </Form>
        );
    }
};
