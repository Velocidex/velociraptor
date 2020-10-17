import React from 'react';

import NotebooksList from './notebooks-list.js';
import NotebookRenderer from './notebook-renderer.js';

import SplitPane from 'react-split-pane';

import axios from 'axios';
import api from '../core/api-service.js';
import Spinner from '../utils/spinner.js';
import { withRouter }  from "react-router-dom";

// Poll for new notebooks list.
const POLL_TIME = 5000;
const PAGE_SIZE = 100;


class Notebooks extends React.Component {
    state = {
        notebooks: [],
        selected_notebook: {},
        loading: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.interval = setInterval(this.fetchNotebooks, POLL_TIME);
        this.fetchNotebooks();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    fetchNotebooks = () => {
        this.setState({loading: true});
        api.get("v1/GetNotebooks", {
            count: PAGE_SIZE,
            offset: 0,
        }).then(function(response) {
            let notebooks = response.data.items || [];
            let selected_notebook = {};

            // Check the router for a notebook id
            let notebook_id = this.props.match && this.props.match.params &&
                this.props.match.params.notebook_id;
            if (notebook_id) {
                for(var i = 0; i < notebooks.length; i++){
                    if (notebooks[i].notebook_id === notebook_id) {
                        selected_notebook = notebooks[i];
                        break;
                    }
                }
            }

            this.setState({notebooks: notebooks,
                           loading: false,
                           selected_notebook: selected_notebook});
        }.bind(this));
    }

    setSelectedNotebook = (notebook) => {
        this.setState({selected_notebook: notebook});
        this.props.history.push("/notebooks/" + notebook.notebook_id);
    }

    render() {
        return (
            <>
              <Spinner loading={this.state.loading} />
              <SplitPane split="horizontal" defaultSize="30%">
                <NotebooksList
                  fetchNotebooks={this.fetchNotebooks}
                  selected_notebook={this.state.selected_notebook}
                  setSelectedNotebook={this.setSelectedNotebook}
                  notebooks={this.state.notebooks}
                />
                <NotebookRenderer
                  fetchNotebooks={this.fetchNotebooks}
                  notebook={this.state.selected_notebook}
                />
              </SplitPane>
            </>
        );
    }
};

export default withRouter(Notebooks);
