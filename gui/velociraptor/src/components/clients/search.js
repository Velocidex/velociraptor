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

    constructor(props) {
        super(props);
        this.state = {
            // query used to update suggestions
            query: [],
        };

        this.submitForm = this.submitForm.bind(this);
        this.showAll = this.showAll.bind(this);
        this.showSuggestions = this.showSuggestions.bind(this);
        this.setQuery = this.setQuery.bind(this);
    }

    submitForm (e) {
        e.preventDefault();
        let query = this.state.query[0];
        this.props.setSearch(query);
        this.props.history.push('/search?query=' + (query || "all"));
    }

    showAll(e) {
        e.preventDefault();
        this.setState({query:["all"]});
        this.props.setSearch("all");
        this.props.history.push('/search?query=all');
    }


    setQuery(query) {
        if (this.state.query[0] !== query[0]) {
            this.setState({query: query[0]});
            this.props.setSearch(query[0]);
            this.props.history.push('/search?query=' + (query[0] || "all"));
        }
    }

    async showSuggestions(query) {
        this.setState({isLoading: true, query: query});
        api.get('/api/v1/SearchClients', {
            query: query + "*",
            count: 10,
            type:1,

        }).then(resp => {
            if (resp.data && resp.data.names) {
                this.setState({
                    isLoading: false,
                    query: [query],
                    options: resp.data.names,
                });
            }
            return true;
        });
    }

    render() {
        return (
            <Form onSubmit={this.submitForm}>
              <FormGroup>
                <ButtonGroup>
                  <AsyncTypeahead
                    autoFocus={true}
                    defaultInputValue={this.state.query ? this.state.query[0] : ""}
                    placeholder="Search clients"
                    id="searchbox"
                    isLoading={false}
                    options={this.state.options || []}
                    onChange={this.setQuery}
                    onSearch={this.showSuggestions}
                  />
                  <Button id="client_query_submit"
                          onClick={this.submitForm}
                          variant="default" type="submit">
                    <FontAwesomeIcon icon="search"/>
                  </Button>
                  <Button id="show-hosts-btn"
                          onClick={this.showAll}
                          variant="default" type="button">
                    <FontAwesomeIcon icon="server"/>
                  </Button>
                </ButtonGroup>
              </FormGroup>
            </Form>
        );
    }
};


export default withRouter(VeloClientSearch);
