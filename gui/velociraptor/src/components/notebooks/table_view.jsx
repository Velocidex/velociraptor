import _ from 'lodash';
import './table_view.css';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import { withRouter }  from "react-router-dom";
import { JSONparse } from "../utils/json_parse.jsx";
import VeloPagedTable from "../core/paged-table.jsx";


class FullScreenTable extends Component {
    static propTypes = {
        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    }

    componentDidMount = () => {
        if (this.props.match &&
            this.props.match.params &&
            this.props.match.params.state) {
            let state = JSONparse(atob(this.props.match.params.state));
            if (!_.isUndefined(state)) {
                this.setState({
                    params: state.params,
                    url: state.url,
                    env: state.env,
                    formatters: state.formatters,
                });
            }
        };
    }


    state = {
        params: {},
        url: "",
        env: {},
    }

    render() {
        return (
            <div className="fullscreen-table Pane horizontal Pane2 ">
              <VeloPagedTable
                is_fullscreen={true}
                params={this.state.params}
                url={this.state.url}
                env={this.state.env}
                formatters={this.state.formatters}
              />
            </div>
        );
    }
}


export default withRouter(FullScreenTable);
